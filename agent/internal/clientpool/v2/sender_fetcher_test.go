package v2_test

import (
	"io"
	"net"

	"code.cloudfoundry.org/loggregator/agent/internal/clientpool/v2"
	"code.cloudfoundry.org/loggregator/plumbing/v2"
	plumbing "code.cloudfoundry.org/loggregator/plumbing/v2"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("PusherFetcher", func() {
	It("opens a stream with the ingress client", func() {
		server := newSpyIngestorServer(true)
		Expect(server.Start()).To(Succeed())
		defer server.Stop()

		fetcher := v2.NewSenderFetcher(newSpyRegistry(), grpc.WithInsecure())
		closer, sender, err := fetcher.Fetch(server.addr)
		Expect(err).ToNot(HaveOccurred())

		err = sender.Send(&plumbing.EnvelopeBatch{})
		Expect(err).ToNot(HaveOccurred())

		Eventually(server.batch).Should(Receive())
		Expect(closer.Close()).To(Succeed())
	})

	It("opens a stream with the deprecated client when non deprecated is not available", func() {
		server := newSpyIngestorServer(false)
		Expect(server.Start()).To(Succeed())
		defer server.Stop()

		fetcher := v2.NewSenderFetcher(newSpyRegistry(), grpc.WithInsecure())
		closer, sender, err := fetcher.Fetch(server.addr)
		Expect(err).ToNot(HaveOccurred())

		err = sender.Send(&plumbing.EnvelopeBatch{})
		Expect(err).ToNot(HaveOccurred())

		Eventually(server.deprecatedBatch).Should(Receive())
		Expect(closer.Close()).To(Succeed())
	})

	It("increments a counter when a connection is established", func() {
		server := newSpyIngestorServer(true)
		Expect(server.Start()).To(Succeed())
		defer server.Stop()

		registry := newSpyRegistry()

		fetcher := v2.NewSenderFetcher(registry, grpc.WithInsecure())
		_, _, err := fetcher.Fetch(server.addr)
		Expect(err).ToNot(HaveOccurred())

		Expect(registry.GetValue("dopplerConnections")).To(Equal(int64(1)))
		Expect(registry.GetValue("dopplerV2Streams")).To(Equal(int64(1)))
	})

	It("decrements a counter when a connection is closed", func() {
		server := newSpyIngestorServer(true)
		Expect(server.Start()).To(Succeed())
		defer server.Stop()

		registry := newSpyRegistry()

		fetcher := v2.NewSenderFetcher(registry, grpc.WithInsecure())
		closer, _, err := fetcher.Fetch(server.addr)
		Expect(err).ToNot(HaveOccurred())

		closer.Close()
		Expect(registry.GetValue("dopplerConnections")).To(Equal(int64(0)))
		Expect(registry.GetValue("dopplerV2Streams")).To(Equal(int64(0)))
	})

	It("returns an error when the server is unavailable", func() {
		fetcher := v2.NewSenderFetcher(newSpyRegistry(), grpc.WithInsecure())
		_, _, err := fetcher.Fetch("127.0.0.1:1122")
		Expect(err).To(HaveOccurred())
	})
})

type SpyRegistry struct {
	counters map[string]int64
}

func newSpyRegistry() *SpyRegistry {
	return &SpyRegistry{
		counters: make(map[string]int64),
	}
}

func (s *SpyRegistry) Inc(name string) {
	s.counters[name] += 1
}

func (s *SpyRegistry) Dec(name string) {
	s.counters[name] -= 1
}

func (s *SpyRegistry) GetValue(name string) int64 {
	v, ok := s.counters[name]
	if !ok {
		return -89282828
	}

	return v
}

type SpyIngestorServer struct {
	addr             string
	server           *grpc.Server
	stop             chan struct{}
	deprecatedBatch  chan *plumbing.EnvelopeBatch
	batch            chan *plumbing.EnvelopeBatch
	includeV2Ingress bool
}

func newSpyIngestorServer(includeV2Ingress bool) *SpyIngestorServer {
	return &SpyIngestorServer{
		stop:             make(chan struct{}),
		batch:            make(chan *plumbing.EnvelopeBatch),
		deprecatedBatch:  make(chan *plumbing.EnvelopeBatch),
		includeV2Ingress: includeV2Ingress,
	}
}

func (s *SpyIngestorServer) Start() error {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}

	s.server = grpc.NewServer()
	s.addr = lis.Addr().String()
	plumbing.RegisterDopplerIngressServer(s.server, &spyV2DeprecatedIngressServer{s})

	if s.includeV2Ingress {
		plumbing.RegisterIngressServer(s.server, &spyV2IngressServer{s})
	}

	go s.server.Serve(lis)

	return nil
}

func (s *SpyIngestorServer) Stop() {
	close(s.stop)
	s.server.Stop()
}

type spyV2DeprecatedIngressServer struct {
	spyIngestorServer *SpyIngestorServer
}

func (s *spyV2DeprecatedIngressServer) Sender(srv plumbing.DopplerIngress_SenderServer) error {
	return nil
}

func (s *spyV2DeprecatedIngressServer) BatchSender(srv plumbing.DopplerIngress_BatchSenderServer) error {
	for {
		select {
		case <-s.spyIngestorServer.stop:
			break
		default:
			b, err := srv.Recv()
			if err != nil {
				break
			}

			s.spyIngestorServer.deprecatedBatch <- b
		}
	}
}

type spyV2IngressServer struct {
	spyIngestorServer *SpyIngestorServer
}

func (s *spyV2IngressServer) Send(context.Context, *loggregator_v2.EnvelopeBatch) (*loggregator_v2.SendResponse, error) {
	return nil, nil
}

func (s *spyV2IngressServer) Sender(srv plumbing.Ingress_SenderServer) error {
	return nil
}

func (s *spyV2IngressServer) BatchSender(srv plumbing.Ingress_BatchSenderServer) error {
	for {
		select {
		case <-srv.Context().Done():
			return nil
		case <-s.spyIngestorServer.stop:
			return io.EOF
		default:
			b, err := srv.Recv()
			if err != nil {
				return nil
			}

			s.spyIngestorServer.batch <- b
		}
	}
}
