package main

import (
	"io/ioutil"
	"log"
	"math/rand"
	"time"

	"code.cloudfoundry.org/loggregator/profiler"
	"code.cloudfoundry.org/loggregator/router/app"
	"google.golang.org/grpc/grpclog"
)

func main() {
	rand.Seed(time.Now().UnixNano())
	grpclog.SetLogger(log.New(ioutil.Discard, "", 0))

	conf, err := app.LoadConfig()
	if err != nil {
		log.Fatalf("Unable to parse config: %s", err)
	}

	r := app.NewRouter(
		conf.GRPC,
		app.WithPersistence(
			conf.MaxRetainedLogMessages,
			conf.ContainerMetricTTLSeconds,
			conf.SinkInactivityTimeoutSeconds,
		),
		app.WithMetricReporting(
			conf.HealthAddr,
			conf.Agent,
			conf.MetricBatchIntervalMilliseconds,
			conf.MetricSourceID,
		),
	)
	r.Start()

	profiler.New(conf.PProfPort).Start()
}
