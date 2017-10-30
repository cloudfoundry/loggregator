package app

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"code.cloudfoundry.org/loggregator/healthendpoint"

	"code.cloudfoundry.org/loggregator/metricemitter"
	"code.cloudfoundry.org/loggregator/plumbing"
	"code.cloudfoundry.org/loggregator/profiler"
	"code.cloudfoundry.org/loggregator/trafficcontroller/internal/auth"
	"code.cloudfoundry.org/loggregator/trafficcontroller/internal/proxy"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

// MetricClient can be used to emit metrics and events.
type MetricClient interface {
	NewCounter(name string, opts ...metricemitter.MetricOption) *metricemitter.Counter
	NewGauge(name string, unit string, opts ...metricemitter.MetricOption) *metricemitter.Gauge
	EmitEvent(title string, body string)
}

type TrafficController struct {
	conf                 *Config
	disableAccessControl bool
	metricClient         MetricClient
	uaaHTTPClient        *http.Client
	ccHTTPClient         *http.Client
}

// finder provides service discovery of Doppler processes
type finder interface {
	Start()
	Next() plumbing.Event
}

func NewTrafficController(
	c *Config,
	disableAccessControl bool,
	metricClient MetricClient,
	uaaHTTPClient *http.Client,
	ccHTTPClient *http.Client,
) *TrafficController {
	return &TrafficController{
		conf:                 c,
		disableAccessControl: disableAccessControl,
		metricClient:         metricClient,
		uaaHTTPClient:        uaaHTTPClient,
		ccHTTPClient:         ccHTTPClient,
	}
}

func (t *TrafficController) Start() {
	log.Print("Startup: Setting up the loggregator traffic controller")

	logAuthorizer := auth.NewLogAccessAuthorizer(
		t.ccHTTPClient,
		t.disableAccessControl,
		t.conf.ApiHost,
	)

	uaaClient := auth.NewUaaClient(
		t.uaaHTTPClient,
		t.conf.UaaHost,
		t.conf.UaaClient,
		t.conf.UaaClientSecret,
	)
	adminAuthorizer := auth.NewAdminAccessAuthorizer(t.disableAccessControl, &uaaClient)

	// Start the health endpoint listener
	promRegistry := prometheus.NewRegistry()
	healthendpoint.StartServer(t.conf.HealthAddr, promRegistry)
	healthRegistry := healthendpoint.New(promRegistry, map[string]prometheus.Gauge{
		// metric-documentation-health: (firehoseStreamCount)
		// Number of open firehose streams
		"firehoseStreamCount": prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "loggregator",
				Subsystem: "trafficcontroller",
				Name:      "firehoseStreamCount",
				Help:      "Number of open firehose streams",
			},
		),
		// metric-documentation-health: (appStreamCount)
		// Number of open app streams
		"appStreamCount": prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "loggregator",
				Subsystem: "trafficcontroller",
				Name:      "appStreamCount",
				Help:      "Number of open app streams",
			},
		),
		// metric-documentation-health: (slowConsumerCount)
		// Number of stream consumers disconnected to avoid backpressure on
		// the Loggregator system.
		"slowConsumerCount": prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "loggregator",
				Subsystem: "trafficcontroller",
				Name:      "slowConsumerCount",
				Help:      "Number of stream consumers disconnected to avoid backpressure on the Loggregator system",
			},
		),
	})

	creds, err := plumbing.NewClientCredentials(
		t.conf.GRPC.CertFile,
		t.conf.GRPC.KeyFile,
		t.conf.GRPC.CAFile,
		"doppler",
	)
	if err != nil {
		log.Fatalf("Could not use GRPC creds for server: %s", err)
	}

	f := plumbing.NewStaticFinder(t.conf.RouterAddrs)
	f.Start()

	kp := keepalive.ClientParameters{
		Time:                15 * time.Second,
		Timeout:             20 * time.Second,
		PermitWithoutStream: true,
	}
	pool := plumbing.NewPool(20, grpc.WithTransportCredentials(creds), grpc.WithKeepaliveParams(kp))
	grpcConnector := plumbing.NewGRPCConnector(1000, pool, f, t.metricClient)

	dopplerHandler := http.Handler(
		proxy.NewDopplerProxy(
			logAuthorizer,
			adminAuthorizer,
			grpcConnector,
			"doppler."+t.conf.SystemDomain,
			5*time.Second,
			5*time.Second,
			t.metricClient,
			healthRegistry,
		),
	)

	var accessMiddleware func(http.Handler) *auth.AccessHandler
	if t.conf.SecurityEventLog != "" {
		accessLog, err := os.OpenFile(t.conf.SecurityEventLog, os.O_APPEND|os.O_WRONLY, os.ModeAppend)
		if err != nil {
			log.Panicf("Unable to open access log: %s", err)
		}
		defer func() {
			accessLog.Sync()
			accessLog.Close()
		}()
		accessLogger := auth.NewAccessLogger(accessLog)
		accessMiddleware = auth.Access(accessLogger, t.conf.IP, t.conf.OutgoingDropsondePort)
	}

	if accessMiddleware != nil {
		dopplerHandler = accessMiddleware(dopplerHandler)
	}
	go func() {
		lis, err := net.Listen("tcp", fmt.Sprintf(":%d", t.conf.OutgoingDropsondePort))
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("ws bound to: %s", lis.Addr())
		log.Fatal(http.Serve(lis, dopplerHandler))
	}()

	// We start the profiler last so that we can definitively claim that we're ready for
	// connections by the time we're listening on the PPROFPort.
	p := profiler.New(t.conf.PProfPort)
	go p.Start()

	killChan := make(chan os.Signal)
	signal.Notify(killChan, os.Interrupt)
	<-killChan
	log.Print("Shutting down")
}
