package v2

import (
	"errors"
	"io"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
)

type ClientFetcher interface {
	Fetch(addr string) (conn io.Closer, client loggregator_v2.Ingress_BatchSenderClient, err error)
}

type GRPCConnector struct {
	fetcher   ClientFetcher
	balancers []*Balancer
}

func MakeGRPCConnector(fetcher ClientFetcher, balancers []*Balancer) GRPCConnector {
	return GRPCConnector{
		fetcher:   fetcher,
		balancers: balancers,
	}
}

func (c GRPCConnector) Connect() (io.Closer, loggregator_v2.Ingress_BatchSenderClient, error) {
	for _, balancer := range c.balancers {
		hostPort, err := balancer.NextHostPort()
		if err != nil {
			continue
		}

		return c.fetcher.Fetch(hostPort)
	}

	return nil, nil, errors.New("unable to lookup a log consumer")
}
