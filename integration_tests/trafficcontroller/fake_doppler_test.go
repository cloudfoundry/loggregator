package trafficcontroller_test

import (
	"net"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator/plumbing"
	"code.cloudfoundry.org/loggregator/testservers"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type FakeDoppler struct {
	GrpcEndpoint        string
	grpcListener        net.Listener
	v2GRPCOut           chan *loggregator_v2.Envelope
	grpcServer          *grpc.Server
	EgressBatchRequests chan *loggregator_v2.EgressBatchRequest
	EgressBatchStreams  chan loggregator_v2.Egress_BatchedReceiverServer
	done                chan struct{}
}

func NewFakeDoppler() *FakeDoppler {
	return &FakeDoppler{
		GrpcEndpoint:        "127.0.0.1:0",
		v2GRPCOut:           make(chan *loggregator_v2.Envelope, 100),
		EgressBatchRequests: make(chan *loggregator_v2.EgressBatchRequest, 100),
		EgressBatchStreams:  make(chan loggregator_v2.Egress_BatchedReceiverServer, 100),
		done:                make(chan struct{}),
	}
}

func (fakeDoppler *FakeDoppler) Addr() string {
	return fakeDoppler.grpcListener.Addr().String()
}

func (fakeDoppler *FakeDoppler) Start() error {
	var err error
	fakeDoppler.grpcListener, err = net.Listen("tcp", fakeDoppler.GrpcEndpoint)
	if err != nil {
		return err
	}
	tlsConfig, err := plumbing.NewServerMutualTLSConfig(
		testservers.Cert("reverselogproxy.crt"),
		testservers.Cert("reverselogproxy.key"),
		testservers.Cert("loggregator-ca.crt"),
	)
	if err != nil {
		return err
	}
	transportCreds := credentials.NewTLS(tlsConfig)

	fakeDoppler.grpcServer = grpc.NewServer(grpc.Creds(transportCreds))
	loggregator_v2.RegisterEgressServer(fakeDoppler.grpcServer, fakeDoppler)

	go func() {
		defer close(fakeDoppler.done)
		fakeDoppler.grpcServer.Serve(fakeDoppler.grpcListener)
	}()
	return nil
}

func (fakeDoppler *FakeDoppler) Stop() {
	if fakeDoppler.grpcServer != nil {
		fakeDoppler.grpcServer.Stop()
	}
	<-fakeDoppler.done
}

func (fakeDoppler *FakeDoppler) SendV2LogMessage(sourceID, payload string) {
	fakeDoppler.v2GRPCOut <- &loggregator_v2.Envelope{
		SourceId: sourceID,
		Message: &loggregator_v2.Envelope_Log{
			Log: &loggregator_v2.Log{
				Payload: []byte(payload),
			},
		},
	}
}

func (fakeDoppler *FakeDoppler) CloseLogMessageStream() {
	close(fakeDoppler.v2GRPCOut)
}

func (fakeDoppler *FakeDoppler) Receiver(*loggregator_v2.EgressRequest, loggregator_v2.Egress_ReceiverServer) error {
	panic("not implemented")
}

func (fakeDoppler *FakeDoppler) BatchedReceiver(
	req *loggregator_v2.EgressBatchRequest,
	stream loggregator_v2.Egress_BatchedReceiverServer,
) error {
	fakeDoppler.EgressBatchRequests <- req
	fakeDoppler.EgressBatchStreams <- stream

	for env := range fakeDoppler.v2GRPCOut {
		stream.Send(&loggregator_v2.EnvelopeBatch{
			Batch: []*loggregator_v2.Envelope{env},
		})
	}

	return nil
}
