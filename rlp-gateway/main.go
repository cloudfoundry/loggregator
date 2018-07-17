package main

import (
	"log"

	"code.cloudfoundry.org/loggregator/rlp-gateway/app"
)

func main() {
	log.Println("starting RLP gateway")
	defer log.Println("stopping RLP gateway")

	app.NewGateway(app.LoadConfig()).Start(true)
}
