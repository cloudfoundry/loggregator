package proxy_test

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"testing"

	loggregator "code.cloudfoundry.org/go-loggregator"
	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator/plumbing"
	"code.cloudfoundry.org/loggregator/trafficcontroller/internal/proxy"

	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestProxy(t *testing.T) {
	grpclog.SetLogger(log.New(GinkgoWriter, "", log.LstdFlags))
	log.SetOutput(GinkgoWriter)
	RegisterFailHandler(Fail)
	RunSpecs(t, "Proxy Suite")
}

type AuthorizerResult struct {
	Status       int
	ErrorMessage string
}

type LogAuthorizer struct {
	TokenParam string
	Target     string
	Result     AuthorizerResult
}

var _ = BeforeSuite(func() {
	proxy.MetricsInterval = 100 * time.Millisecond
})

func (a *LogAuthorizer) Authorize(authToken string, target string) (int, error) {
	a.TokenParam = authToken
	a.Target = target

	return a.Result.Status, errors.New(a.Result.ErrorMessage)
}

type AdminAuthorizer struct {
	TokenParam string
	Result     AuthorizerResult
}

func (a *AdminAuthorizer) Authorize(authToken string) (bool, error) {
	a.TokenParam = authToken

	return a.Result.Status == http.StatusOK, errors.New(a.Result.ErrorMessage)
}

func startListener(addr string) net.Listener {
	var lis net.Listener
	f := func() error {
		var err error
		lis, err = net.Listen("tcp", addr)
		return err
	}
	Eventually(f).ShouldNot(HaveOccurred())

	return lis
}

func startGRPCServer(ds plumbing.DopplerServer, addr string) (net.Listener, *grpc.Server) {
	lis := startListener(addr)
	s := grpc.NewServer()
	plumbing.RegisterDopplerServer(s, ds)
	go s.Serve(lis)

	return lis, s
}

type subscribeRequest struct {
	ctx     context.Context
	request *loggregator_v2.EgressBatchRequest
}

type SpyGRPCConnector struct {
	mu               sync.Mutex
	subscriptions    *subscribeRequest
	subscriptionsErr error
}

func newSpyGRPCConnector(err error) *SpyGRPCConnector {
	return &SpyGRPCConnector{
		subscriptionsErr: err,
	}
}

func (s *SpyGRPCConnector) Stream(
	ctx context.Context,
	req *loggregator_v2.EgressBatchRequest,
) loggregator.EnvelopeStream {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subscriptions = &subscribeRequest{
		ctx:     ctx,
		request: req,
	}

	return loggregator.EnvelopeStream(func() []*loggregator_v2.Envelope {
		if s.subscriptionsErr != nil {
			return nil
		}

		return []*loggregator_v2.Envelope{
			{
				SourceId: "abc-123",
				Message: &loggregator_v2.Envelope_Log{
					Log: &loggregator_v2.Log{
						Payload: []byte("hello"),
					},
				},
			},
		}
	})
}
