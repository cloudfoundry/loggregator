package app

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"time"

	"code.cloudfoundry.org/loggregator/agent/internal/clientpool"
	clientpoolv1 "code.cloudfoundry.org/loggregator/agent/internal/clientpool/v1"
	egress "code.cloudfoundry.org/loggregator/agent/internal/egress/v1"
	ingress "code.cloudfoundry.org/loggregator/agent/internal/ingress/v1"
	"code.cloudfoundry.org/loggregator/healthendpoint"
	"code.cloudfoundry.org/loggregator/metricemitter"
	"code.cloudfoundry.org/loggregator/plumbing"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

type AppV1 struct {
	config          *Config
	creds           credentials.TransportCredentials
	healthRegistrar *healthendpoint.Registrar
	metricClient    MetricClient
	lookup          func(string) ([]net.IP, error)
}

// AppV1Option configures AppV1 options.
type AppV1Option func(*AppV1)

// WithV1Lookup allows the default DNS resolver to be changed.
func WithV1Lookup(l func(string) ([]net.IP, error)) func(*AppV1) {
	return func(a *AppV1) {
		a.lookup = l
	}
}

func NewV1App(
	c *Config,
	r *healthendpoint.Registrar,
	creds credentials.TransportCredentials,
	m MetricClient,
	opts ...AppV1Option,
) *AppV1 {
	a := &AppV1{
		config:          c,
		healthRegistrar: r,
		creds:           creds,
		metricClient:    m,
		lookup:          net.LookupIP,
	}

	for _, o := range opts {
		o(a)
	}

	return a
}

func (a *AppV1) Start() {
	if a.config.DisableUDP {
		return
	}

	eventWriter := egress.New("MetronAgent")

	log.Print("Startup: Setting up the agent")
	marshaller := a.initializeV1DopplerPool()

	messageTagger := egress.NewTagger(
		a.config.Deployment,
		a.config.Job,
		a.config.Index,
		a.config.IP,
		marshaller,
	)
	aggregator := egress.NewAggregator(messageTagger)
	eventWriter.SetWriter(aggregator)

	dropsondeUnmarshaller := ingress.NewUnMarshaller(aggregator)
	agentAddress := fmt.Sprintf("127.0.0.1:%d", a.config.IncomingUDPPort)
	networkReader, err := ingress.NewNetworkReader(
		agentAddress,
		dropsondeUnmarshaller,
		a.metricClient,
	)
	if err != nil {
		log.Panic(fmt.Errorf("Failed to listen on %s: %s", agentAddress, err))
	}

	log.Printf("agent v1 API started on addr %s", agentAddress)
	go networkReader.StartReading()
	networkReader.StartWriting()
}

func (a *AppV1) initializeV1DopplerPool() *egress.EventMarshaller {
	pool := a.setupGRPC()

	marshaller := egress.NewMarshaller(a.metricClient)
	marshaller.SetWriter(pool)

	return marshaller
}

func (a *AppV1) setupGRPC() *clientpoolv1.ClientPool {
	if a.creds == nil {
		return nil
	}

	balancers := make([]*clientpoolv1.Balancer, 0, 2)

	if a.config.RouterAddrWithAZ != "" {
		balancers = append(balancers, clientpoolv1.NewBalancer(
			a.config.RouterAddrWithAZ,
			clientpoolv1.WithLookup(a.lookup),
		))
	}
	balancers = append(balancers, clientpoolv1.NewBalancer(
		a.config.RouterAddr,
		clientpoolv1.WithLookup(a.lookup),
	))

	avgEnvelopeSize := a.metricClient.NewGauge("average_envelope", "bytes/minute",
		metricemitter.WithVersion(2, 0),
		metricemitter.WithTags(map[string]string{
			"loggregator": "v1",
		}))
	tracker := plumbing.NewEnvelopeAverager()
	tracker.Start(60*time.Second, func(average float64) {
		avgEnvelopeSize.Set(average)
	})
	statsHandler := clientpool.NewStatsHandler(tracker)

	kp := keepalive.ClientParameters{
		Time:                15 * time.Second,
		Timeout:             15 * time.Second,
		PermitWithoutStream: true,
	}
	fetcher := clientpoolv1.NewPusherFetcher(
		a.healthRegistrar,
		grpc.WithTransportCredentials(a.creds),
		grpc.WithStatsHandler(statsHandler),
		grpc.WithKeepaliveParams(kp),
	)

	connector := clientpoolv1.MakeGRPCConnector(fetcher, balancers)

	var connManagers []clientpoolv1.Conn
	for i := 0; i < 5; i++ {
		connManagers = append(connManagers, clientpoolv1.NewConnManager(
			connector,
			100000+rand.Int63n(1000),
			time.Second,
		))
	}

	return clientpoolv1.New(connManagers...)
}
