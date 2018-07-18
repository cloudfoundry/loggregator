package ingress

import (
	"context"
	"log"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	throughputlb "code.cloudfoundry.org/grpc-throughputlb"
	"code.cloudfoundry.org/loggregator/rlp-gateway/internal/web"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type LogClient struct {
	c loggregator_v2.EgressClient
}

func NewLogClient(creds credentials.TransportCredentials, logsProviderAddr string) *LogClient {
	conn, err := grpc.Dial(logsProviderAddr,
		grpc.WithTransportCredentials(creds),
		grpc.WithBalancer(throughputlb.NewThroughputLoadBalancer(100, 20)),
		grpc.WithBlock(),
	)
	if err != nil {
		log.Fatalf("failed to dial logs provider: %s", err)
	}

	client := loggregator_v2.NewEgressClient(conn)
	return &LogClient{
		c: client,
	}
}

func (c *LogClient) Stream(ctx context.Context, req *loggregator_v2.EgressBatchRequest) web.Receiver {
	receiver, err := c.c.BatchedReceiver(ctx, req)
	if err != nil {
		log.Fatalf("failed to open stream from logs provider: %s", err) // TODO: Do not log fatal, this could actually happen a lot
	}

	return receiver.Recv
}
