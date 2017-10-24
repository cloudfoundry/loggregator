package app

import (
	"log"
	"net"
	"time"

	gendiodes "code.cloudfoundry.org/go-diodes"
	"code.cloudfoundry.org/loggregator/diodes"
	"code.cloudfoundry.org/loggregator/healthendpoint"
	"code.cloudfoundry.org/loggregator/metricemitter"
	"code.cloudfoundry.org/loggregator/plumbing"
	"code.cloudfoundry.org/loggregator/router/internal/server"
	"code.cloudfoundry.org/loggregator/router/internal/server/v1"
	"code.cloudfoundry.org/loggregator/router/internal/server/v2"
	"code.cloudfoundry.org/loggregator/router/internal/sinks"
	"github.com/cloudfoundry/dropsonde"
	"github.com/cloudfoundry/dropsonde/metric_sender"
	"github.com/cloudfoundry/dropsonde/metricbatcher"
	"github.com/cloudfoundry/dropsonde/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

// Router routes envelopes from producers to any subscribers.
type Router struct {
	c              *Config
	healthListener net.Listener
	server         *server.Server
	addrs          Addrs
}

// NewLegacyRouter creates a new Router with the given config.
// Using the config for construction is deprecated and will be removed once
// syslog and etcd are removed.
func NewLegacyRouter(c *Config) *Router {
	return &Router{
		c: c,
	}
}

// NewRouter creates a new Router with the given options. Each provided
// RouterOption will manipulate the Router behavior.
func NewRouter(grpc GRPC, opts ...RouterOption) *Router {
	d := &Router{
		c: &Config{
			GRPC: grpc,
			MetricBatchIntervalMilliseconds: 5000,
			Agent: Agent{
				UDPAddress:  "127.0.0.1:3457",
				GRPCAddress: "127.0.0.1:3458",
			},
			HealthAddr:                "localhost:14825",
			MaxRetainedLogMessages:    100,
			MessageDrainBufferSize:    10000,
			ContainerMetricTTLSeconds: 120,
		},
	}

	for _, o := range opts {
		o(d)
	}

	return d
}

// RouterOption is used to configure a new Router.
type RouterOption func(*Router)

// WithMetricReporting returns a RouterOption that enables Router to emit
// metrics about itself.
// This option is experimental.
func WithMetricReporting() RouterOption {
	return func(d *Router) {
		panic("Not yet implemented")
	}
}

// WithPersistence turns on recent logs and container metric storage.
// This option is experimental.
func WithPersistence() RouterOption {
	return func(d *Router) {
		panic("Not yet implemented")
	}
}

// Start enables the Router to start receiving envelope, accepting
// subscriptions and routing data.
func (d *Router) Start() {
	log.Printf("Startup: Setting up the router server")

	//------------------------------
	// v1 Metrics (UDP)
	//------------------------------
	metricBatcher := initV1Metrics(
		d.c.MetricBatchIntervalMilliseconds,
		d.c.Agent.UDPAddress,
	)

	//------------------------------
	// v2 Metrics (gRPC)
	//------------------------------
	metricClient := initV2Metrics(d.c)

	//------------------------------
	// Health
	//------------------------------
	promRegistry := prometheus.NewRegistry()
	d.healthListener = healthendpoint.StartServer(d.c.HealthAddr, promRegistry)
	d.addrs.Health = d.healthListener.Addr().String()
	healthRegistrar := initHealthRegistrar(promRegistry)

	//------------------------------
	// In memory store of
	// - recent logs
	// - container metrics
	//------------------------------
	sinkManager := sinks.NewSinkManager(
		d.c.MaxRetainedLogMessages,
		d.c.MessageDrainBufferSize,
		"DopplerServer",
		time.Duration(d.c.SinkInactivityTimeoutSeconds)*time.Second,
		time.Duration(d.c.ContainerMetricTTLSeconds)*time.Second,
		metricBatcher,
		metricClient,
		healthRegistrar,
	)

	//------------------------------
	// Ingress (gRPC v1 and v2)
	// Egress  (gRPC v1 and v2)
	//------------------------------
	droppedMetric := metricClient.NewCounter(
		"dropped",
		metricemitter.WithVersion(2, 0),
		metricemitter.WithTags(map[string]string{"direction": "ingress"}),
	)

	v1Buf := diodes.NewManyToOneEnvelope(10000, gendiodes.AlertFunc(func(missed int) {
		log.Printf("Shed %d envelopes (v1 buffer)", missed)

		// metric-documentation-v1: (doppler.shedEnvelopes) Number of envelopes dropped by the
		// diode inbound from metron
		metricBatcher.BatchCounter("doppler.shedEnvelopes").Add(uint64(missed))
	}))

	v2Buf := diodes.NewManyToOneEnvelopeV2(10000, gendiodes.AlertFunc(func(missed int) {
		log.Printf("Dropped %d envelopes (v2 buffer)", missed)

		// metric-documentation-v2: (loggregator.doppler.dropped) Number of envelopes dropped by the
		// diode inbound from metron
		droppedMetric.Increment(uint64(missed))
	}))

	v1Ingress := v1.NewIngestorServer(
		v1Buf,
		v2Buf,
		metricBatcher,
		healthRegistrar,
	)
	v1Router := v1.NewRouter()
	v1Egress := v1.NewDopplerServer(
		v1Router,
		sinkManager,
		metricClient,
		healthRegistrar,
		time.Second,
		100,
	)
	v2Ingress := v2.NewIngressServer(
		v1Buf,
		v2Buf,
		metricBatcher,
		metricClient,
		healthRegistrar,
	)
	v2PubSub := v2.NewPubSub()
	v2Egress := v2.NewEgressServer(
		v2PubSub,
		healthRegistrar,
		time.Second,
		100,
	)

	var opts []plumbing.ConfigOption
	if len(d.c.GRPC.CipherSuites) > 0 {
		opts = append(opts, plumbing.WithCipherSuites(d.c.GRPC.CipherSuites))
	}
	tlsConfig, err := plumbing.NewServerMutualTLSConfig(
		d.c.GRPC.CertFile,
		d.c.GRPC.KeyFile,
		d.c.GRPC.CAFile,
		opts...,
	)
	if err != nil {
		log.Panicf("Failed to create tls config for router server: %s", err)
	}
	srv, err := server.NewServer(
		d.c.GRPC.Port,
		v1Ingress,
		v1Egress,
		v2Ingress,
		v2Egress,
		grpc.Creds(credentials.NewTLS(tlsConfig)),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             10 * time.Second,
			PermitWithoutStream: true,
		}),
	)
	if err != nil {
		log.Panicf("Failed to create router server: %s", err)
	}

	d.server = srv
	d.addrs.GRPC = d.server.Addr()

	//------------------------------
	// Start
	//------------------------------
	messageRouter := sinks.NewMessageRouter(sinkManager, v1Router)
	go messageRouter.Start(v1Buf)

	repeater := v2.NewRepeater(v2PubSub.Publish, v2Buf.Next)
	go repeater.Start()

	go d.server.Start()

	log.Print("Startup: router server started.")
}

// Addrs stores listener addresses of the router process.
type Addrs struct {
	GRPC   string
	Health string
}

// Addrs returns a copy of the listeners' addresses.
func (d *Router) Addrs() Addrs {
	return d.addrs
}

// Stop closes the gRPC and health listeners.
func (d *Router) Stop() {
	// TODO: Drain
	d.healthListener.Close()
	d.server.Stop()
}

func initV1Metrics(milliseconds uint, udpAddr string) *metricbatcher.MetricBatcher {
	err := dropsonde.Initialize(udpAddr, "DopplerServer")
	if err != nil {
		log.Fatal(err)
	}
	eventEmitter := dropsonde.AutowiredEmitter()
	metricSender := metric_sender.NewMetricSender(eventEmitter)
	metricBatcher := metricbatcher.New(
		metricSender,
		time.Duration(milliseconds)*time.Millisecond,
	)
	metricBatcher.AddConsistentlyEmittedMetrics(
		"doppler.shedEnvelopes",
		"TruncatingBuffer.totalDroppedMessages",
		"listeners.totalReceivedMessageCount",
	)
	metrics.Initialize(metricSender, metricBatcher)
	return metricBatcher
}

func initV2Metrics(c *Config) *metricemitter.Client {
	credentials, err := plumbing.NewClientCredentials(
		c.GRPC.CertFile,
		c.GRPC.KeyFile,
		c.GRPC.CAFile,
		"metron",
	)
	if err != nil {
		log.Fatalf("Could not use GRPC creds for server: %s", err)
	}

	batchInterval := time.Duration(c.MetricBatchIntervalMilliseconds) * time.Millisecond

	// metric-documentation-v2: setup function
	metricClient, err := metricemitter.NewClient(
		c.Agent.GRPCAddress,
		metricemitter.WithGRPCDialOptions(grpc.WithTransportCredentials(credentials)),
		metricemitter.WithOrigin("loggregator.doppler"),
		metricemitter.WithPulseInterval(batchInterval),
	)
	if err != nil {
		log.Fatalf("Could not configure metric emitter: %s", err)
	}

	return metricClient
}

func initHealthRegistrar(r prometheus.Registerer) *healthendpoint.Registrar {
	return healthendpoint.New(r, map[string]prometheus.Gauge{
		// metric-documentation-health: (ingressStreamCount)
		// Number of open firehose streams
		"ingressStreamCount": prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "loggregator",
				Subsystem: "router",
				Name:      "ingressStreamCount",
				Help:      "Number of open ingress streams",
			},
		),
		// metric-documentation-health: (subscriptionCount)
		// Number of open subscriptions
		"subscriptionCount": prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "loggregator",
				Subsystem: "router",
				Name:      "subscriptionCount",
				Help:      "Number of open subscriptions",
			},
		),
		// metric-documentation-health: (recentLogCacheCount)
		// Number of recent log caches
		"recentLogCacheCount": prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "loggregator",
				Subsystem: "router",
				Name:      "recentLogCacheCount",
				Help:      "Number of recent log caches",
			},
		),
		// metric-documentation-health: (containerMetricCacheCount)
		// Number of container metric caches
		"containerMetricCacheCount": prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "loggregator",
				Subsystem: "router",
				Name:      "containerMetricCacheCount",
				Help:      "Number of container metric caches",
			},
		),
	})
}
