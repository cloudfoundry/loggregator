package ingress

import (
	"context"
	"log"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator/rlp-gateway/internal/web"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type LogClient struct {
	c loggregator_v2.EgressClient
}

func NewLogClient(creds credentials.TransportCredentials, logsProviderAddr string) *LogClient {
	conn, err := grpc.Dial(logsProviderAddr, grpc.WithTransportCredentials(creds))
	if err != nil {
		log.Fatalf("failed to connect to logs provider: %s", err)
	}

	client := loggregator_v2.NewEgressClient(conn)
	return &LogClient{
		c: client,
	}
}

func (c *LogClient) Stream(ctx context.Context, req *loggregator_v2.EgressBatchRequest) web.Receiver {
	receiver, err := c.c.BatchedReceiver(ctx, req)
	if err != nil {
		log.Fatalf("failed to connect to logs provider: %s", err)
	}

	return receiver.Recv
}
