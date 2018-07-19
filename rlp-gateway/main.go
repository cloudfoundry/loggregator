package main

import (
	"log"

	"code.cloudfoundry.org/loggregator/profiler"
	"code.cloudfoundry.org/loggregator/rlp-gateway/app"
)

func main() {
	log.Println("starting RLP gateway")
	defer log.Println("stopping RLP gateway")

	cfg := app.LoadConfig()

	go profiler.New(cfg.PProfPort).Start()
	app.NewGateway(cfg).Start(true)
}
