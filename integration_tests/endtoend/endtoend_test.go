package endtoend_test

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"sync"
	"time"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator/integration_tests/endtoend"
	"code.cloudfoundry.org/loggregator/testservers"
	"github.com/golang/protobuf/jsonpb"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("End to end tests", func() {
	It("sends messages to the rlp-gateway", func() {
		dopplerCleanup, dopplerPorts := testservers.StartRouter(
			testservers.BuildRouterConfig(0, 0),
		)
		defer dopplerCleanup()
		agentCleanup, agentPorts := testservers.StartAgent(
			testservers.BuildAgentConfig("127.0.0.1", dopplerPorts.GRPC),
		)
		defer agentCleanup()
		rlpCleanup, rlpPorts := testservers.StartRLP(
			testservers.BuildRLPConfig(0, agentPorts.GRPC, []string{fmt.Sprintf("127.0.0.1:%d", dopplerPorts.GRPC)}),
		)
		defer rlpCleanup()

		rlpGatewayCleanup, rlpGatewayPorts := testservers.StartRLPGateway(
			testservers.BuildRLPGatewayConfig(0, fmt.Sprintf("127.0.0.1:%d", rlpPorts.GRPC)),
		)
		defer rlpGatewayCleanup()

		client := newTestClient()
		go client.open(fmt.Sprintf("http://127.0.0.1:%d/v2/read?log", rlpGatewayPorts.HTTP))

		go func() {
			agentStreamWriter := endtoend.NewAgentStreamWriter(agentPorts.UDP)
			generator := endtoend.NewLogMessageGenerator("custom-app-id")
			for range time.Tick(time.Millisecond) {
				agentStreamWriter.Write(generator.Generate())
			}
		}()

		Eventually(func() int {
			return len(client.envelopes())
		}).Should(BeNumerically(">", 10))
	})

	It("sends messages from agent through doppler and traffic controller", func() {
		dopplerCleanup, dopplerPorts := testservers.StartRouter(
			testservers.BuildRouterConfig(0, 0),
		)
		defer dopplerCleanup()
		agentCleanup, agentPorts := testservers.StartAgent(
			testservers.BuildAgentConfig("127.0.0.1", dopplerPorts.GRPC),
		)
		defer agentCleanup()
		trafficcontrollerCleanup, tcPorts := testservers.StartTrafficController(
			testservers.BuildTrafficControllerConf(
				dopplerPorts.GRPC,
				agentPorts.UDP,
			),
		)
		defer trafficcontrollerCleanup()

		firehoseReader := endtoend.NewFirehoseReader(tcPorts.WS)

		go func() {
			agentStreamWriter := endtoend.NewAgentStreamWriter(agentPorts.UDP)
			generator := endtoend.NewLogMessageGenerator("custom-app-id")
			for range time.Tick(time.Millisecond) {
				agentStreamWriter.Write(generator.Generate())
			}
		}()

		go func() {
			for {
				firehoseReader.Read()
			}
		}()

		Eventually(firehoseReader.LogMessageAppIDs, 5).Should(Receive(Equal("custom-app-id")))
	}, 10)
})

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
