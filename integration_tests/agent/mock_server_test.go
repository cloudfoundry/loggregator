package agent_test

import (
	"net"
	"time"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator/plumbing"
	"code.cloudfoundry.org/loggregator/testservers"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

type DopplerIngestorServer interface {
	plumbing.DopplerIngestorServer
}

type Server struct {
	port     int
	server   *grpc.Server
	listener net.Listener
	V1       *mockDopplerIngestorServerV1
	V2       *mockIngressServerV2
}

func NewServer() (*Server, error) {
	tlsConfig, err := plumbing.NewServerMutualTLSConfig(
		testservers.Cert("doppler.crt"),
		testservers.Cert("doppler.key"),
		testservers.Cert("loggregator-ca.crt"),
	)
	if err != nil {
		return nil, err
	}
	transportCreds := credentials.NewTLS(tlsConfig)
	mockDopplerV1 := newMockDopplerIngestorServerV1()
	mockDopplerV2 := newMockIngressServerV2()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	s := grpc.NewServer(
		grpc.Creds(transportCreds),
		grpc.KeepaliveEnforcementPolicy(
			keepalive.EnforcementPolicy{
				MinTime:             10 * time.Second,
				PermitWithoutStream: true,
			},
		),
	)
	plumbing.RegisterDopplerIngestorServer(s, mockDopplerV1)
	loggregator_v2.RegisterIngressServer(s, mockDopplerV2)

	go s.Serve(lis)

	return &Server{
		port:     lis.Addr().(*net.TCPAddr).Port,
		server:   s,
		listener: lis,
		V1:       mockDopplerV1,
		V2:       mockDopplerV2,
	}, nil
}

func (s *Server) URI() string {
	return s.listener.Addr().String()
}

func (s *Server) Port() int {
	return s.port
}

func (s *Server) Stop() error {
	err := s.listener.Close()
	s.server.Stop()
	return err
}
