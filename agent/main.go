package main

import (
	"io/ioutil"
	"log"
	"math/rand"
	"time"

	"google.golang.org/grpc/grpclog"

	"code.cloudfoundry.org/loggregator/profiler"

	"code.cloudfoundry.org/loggregator/agent/app"
)

func main() {
	rand.Seed(time.Now().UnixNano())
	grpclog.SetLogger(log.New(ioutil.Discard, "", 0))

	config, err := app.LoadConfig()
	if err != nil {
		log.Fatalf("Unable to parse config: %s", err)
	}

	a := app.NewAgent(config)
	go a.Start()

	// We start the profiler last so that we can definitively say that we're
	// all connected and ready for data by the time the profiler starts up.
	profiler.New(config.PProfPort).Start()
}
