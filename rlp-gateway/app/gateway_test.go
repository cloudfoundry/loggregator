package app_test

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator/plumbing" // TODO: Resolve duplicate proto error
	"code.cloudfoundry.org/loggregator/rlp-gateway/app"
	"code.cloudfoundry.org/loggregator/testservers"
	"google.golang.org/grpc"

	"github.com/gogo/protobuf/jsonpb"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Gateway", func() {
	var (
		logsProvider *stubLogsProvider
		cfg          app.Config
	)

	BeforeEach(func() {
		logsProvider = newStubLogsProvider()
		logsProvider.toSend = 10

		cfg = app.Config{
			LogsProviderAddr: logsProvider.addr(),

			LogsProviderCAPath:         testservers.Cert("loggregator-ca.crt"),
			LogsProviderClientCertPath: testservers.Cert("rlpgateway.crt"),
			LogsProviderClientKeyPath:  testservers.Cert("rlpgateway.key"),
			LogsProviderCommonName:     "reverselogproxy",

			GatewayAddr: ":0",
		}
	})

	It("forwards HTTP requests to RLP", func() {
		gateway := app.NewGateway(cfg)
		gateway.Start(false)
		defer gateway.Stop()

		client := newTestClient()
		go client.open("http://" + gateway.Addr() + "/v2/read?log")

		Eventually(client.envelopes).Should(HaveLen(10))
	})

	It("doesn't panic when the logs provider closes", func() {
		logsProvider.toSend = 1

		gateway := app.NewGateway(cfg)
		gateway.Start(false)
		defer gateway.Stop()

		client := newTestClient()
		Expect(func() {
			client.open("http://" + gateway.Addr() + "/v2/read?log")
		}).ToNot(Panic())
	})
})

type stubLogsProvider struct {
	listener net.Listener
	toSend   int
}

func newStubLogsProvider() *stubLogsProvider {
	creds, err := plumbing.NewServerCredentials(
		testservers.Cert("reverselogproxy.crt"),
		testservers.Cert("reverselogproxy.key"),
		testservers.Cert("loggregator-ca.crt"),
	)
	if err != nil {
		panic(err)
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}

	s := &stubLogsProvider{
		listener: l,
	}

	server := grpc.NewServer(grpc.Creds(creds))
	loggregator_v2.RegisterEgressServer(server, s)

	go func() {
		err := server.Serve(s.listener)
		fmt.Println(err)
	}()

	return s
}

func (s *stubLogsProvider) Receiver(*loggregator_v2.EgressRequest, loggregator_v2.Egress_ReceiverServer) error {
	panic("not implemented")
}

func (s *stubLogsProvider) BatchedReceiver(req *loggregator_v2.EgressBatchRequest, srv loggregator_v2.Egress_BatchedReceiverServer) error {
	for i := 0; i < s.toSend; i++ {
		if isDone(srv.Context()) {
			break
		}

		srv.Send(&loggregator_v2.EnvelopeBatch{
			Batch: []*loggregator_v2.Envelope{
				{Timestamp: time.Now().UnixNano()},
			},
		})
	}

	return nil
}

func (s *stubLogsProvider) addr() string {
	return s.listener.Addr().String()
}

type testClient struct {
	mu         sync.Mutex
	_envelopes []*loggregator_v2.Envelope
}

func newTestClient() *testClient {
	return &testClient{}
}

func (tc *testClient) open(url string) {
	resp, err := http.Get(url)
	if err != nil {
		panic(err)
	}

	if resp.StatusCode != 200 {
		panic(fmt.Sprintf("unhandled status code: %d", resp.StatusCode))
	}

	buf := bytes.NewBuffer(nil)
	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}

		switch {
		case bytes.HasPrefix(line, []byte("data:")):
			buf.Write(line[6:])
		case bytes.Equal(line, []byte("\n")):
			var batch loggregator_v2.EnvelopeBatch
			if err := jsonpb.Unmarshal(buf, &batch); err != nil {
				panic(fmt.Sprintf("failed to unmarshal envelopes: %s", err))
			}
			tc.mu.Lock()
			tc._envelopes = append(tc._envelopes, batch.GetBatch()...)
			tc.mu.Unlock()
		default:
			panic("unhandled SSE data")
		}
	}
}

func (tc *testClient) envelopes() []*loggregator_v2.Envelope {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	return tc._envelopes
}

func isDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}
