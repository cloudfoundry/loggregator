package main

import (
	"flag"
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

	configFile := flag.String(
		"config",
		"config/doppler.json",
		"Location of the doppler config json file",
	)
	flag.Parse()

	conf, err := app.ParseConfig(*configFile)
	if err != nil {
		log.Fatalf("Unable to parse config: %s", err)
	}

	r := app.NewLegacyRouter(conf)
	r.Start()

	p := profiler.New(conf.PPROFPort)
	p.Start()
}
