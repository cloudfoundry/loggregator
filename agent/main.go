package main

import (
	"io/ioutil"
	"log"
	"math/rand"
	"time"

	"code.cloudfoundry.org/loggregator/agent/app"
	"code.cloudfoundry.org/loggregator/profiler"
	"google.golang.org/grpc/grpclog"
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

	profiler.New(config.PProfPort).Start()
}
