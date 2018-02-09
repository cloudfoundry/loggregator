package v2

import (
	"log"
	"net"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"

	"google.golang.org/grpc"
)

type Server struct {
	addr string
	rx   *Receiver
	opts []grpc.ServerOption
}

func NewServer(addr string, rx *Receiver, opts ...grpc.ServerOption) *Server {
	return &Server{
		addr: addr,
		rx:   rx,
		opts: opts,
	}
}

func (s *Server) Start() {
	lis, err := net.Listen("tcp", s.addr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	log.Printf("grpc bound to: %s", lis.Addr())

	grpcServer := grpc.NewServer(s.opts...)
	loggregator_v2.RegisterIngressServer(grpcServer, s.rx)

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
