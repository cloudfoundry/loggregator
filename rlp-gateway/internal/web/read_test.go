package web_test

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator/rlp-gateway/internal/web"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Read", func() {
	var (
		server *httptest.Server
		lp     *stubLogsProvider
		ctx    context.Context
		cancel func()
	)

	BeforeEach(func() {
		lp = newStubLogsProvider()
		lp._batchResponse = &loggregator_v2.EnvelopeBatch{
			Batch: []*loggregator_v2.Envelope{
				{
					SourceId: "source-id-a",
				},
				{
					SourceId: "source-id-b",
				},
			},
		}
		server = httptest.NewServer(web.ReadHandler(lp))
		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() {
		cancel()
		server.CloseClientConnections()
		server.Close()
	})

	It("reads from the logs provider and sends SSE to the client", func() {
		req, err := http.NewRequest(http.MethodGet, server.URL+"/v2/read?log", nil)
		Expect(err).ToNot(HaveOccurred())

		req = req.WithContext(ctx)

		resp, err := server.Client().Do(req)
		Expect(err).ToNot(HaveOccurred())

		Eventually(lp.requests).Should(HaveLen(1))

		Expect(lp.requests()[0]).To(Equal(&loggregator_v2.EgressBatchRequest{
			Selectors: []*loggregator_v2.Selector{
				{
					Message: &loggregator_v2.Selector_Log{
						Log: &loggregator_v2.LogSelector{},
					},
				},
			},
			UsePreferredTags: true,
		}))

		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resp.Header.Get("Content-Type")).To(Equal("text/event-stream"))
		Expect(resp.Header.Get("Cache-Control")).To(Equal("no-cache"))
		Expect(resp.Header.Get("Connection")).To(Equal("keep-alive"))

		buf := bufio.NewReader(resp.Body)

		line, err := buf.ReadBytes('\n')
		Expect(err).ToNot(HaveOccurred())
		Expect(string(line)).To(Equal(`data: {"batch":[{"sourceId":"source-id-a"},{"sourceId":"source-id-b"}]}` + "\n"))

		// Read 1 empty new lines
		_, err = buf.ReadBytes('\n')
		Expect(err).ToNot(HaveOccurred())

		line, err = buf.ReadBytes('\n')
		Expect(err).ToNot(HaveOccurred())
		Expect(string(line)).To(Equal(`data: {"batch":[{"sourceId":"source-id-a"},{"sourceId":"source-id-b"}]}` + "\n"))
	})

	It("adds the shard ID to the egress request", func() {
		req, err := http.NewRequest(
			http.MethodGet,
			server.URL+"/v2/read?log&shard_id=my-shard-id",
			nil,
		)
		Expect(err).ToNot(HaveOccurred())

		req = req.WithContext(ctx)

		_, err = server.Client().Do(req)
		Expect(err).ToNot(HaveOccurred())

		Eventually(lp.requests).Should(HaveLen(1))

		Expect(lp.requests()[0]).To(Equal(&loggregator_v2.EgressBatchRequest{
			ShardId: "my-shard-id",
			Selectors: []*loggregator_v2.Selector{
				{
					Message: &loggregator_v2.Selector_Log{
						Log: &loggregator_v2.LogSelector{},
					},
				},
			},
			UsePreferredTags: true,
		}))
	})

	It("adds the deterministic name to the egress request", func() {
		req, err := http.NewRequest(
			http.MethodGet,
			server.URL+"/v2/read?log&deterministic_name=some-deterministic-name",
			nil,
		)
		Expect(err).ToNot(HaveOccurred())

		req = req.WithContext(ctx)

		_, err = server.Client().Do(req)
		Expect(err).ToNot(HaveOccurred())

		Eventually(lp.requests).Should(HaveLen(1))

		Expect(lp.requests()[0]).To(Equal(&loggregator_v2.EgressBatchRequest{
			DeterministicName: "some-deterministic-name",
			Selectors: []*loggregator_v2.Selector{
				{
					Message: &loggregator_v2.Selector_Log{
						Log: &loggregator_v2.LogSelector{},
					},
				},
			},
			UsePreferredTags: true,
		}))
	})

	It("closes the SSE stream if the envelope stream returns any error", func() {
		lp._batchResponse = nil
		lp._errorResponse = errors.New("an error")

		req, err := http.NewRequest(http.MethodGet, server.URL+"/v2/read?log", nil)
		Expect(err).ToNot(HaveOccurred())

		req = req.WithContext(ctx)

		resp, err := server.Client().Do(req)
		Expect(err).ToNot(HaveOccurred())

		buf := bufio.NewReader(resp.Body)
		Eventually(func() error {
			_, err := buf.ReadBytes('\n')
			return err
		}).Should(Equal(io.EOF))
	})

	It("returns a bad request if no selectors are provided in url", func() {
		req, err := http.NewRequest(http.MethodGet, server.URL+"/v2/read", nil)
		Expect(err).ToNot(HaveOccurred())

		resp, err := server.Client().Do(req.WithContext(ctx))
		Expect(err).ToNot(HaveOccurred())

		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
	})

	It("returns 405, method not allowed", func() {
		req, err := http.NewRequest(http.MethodPost, server.URL+"/v2/read?log", nil)
		Expect(err).ToNot(HaveOccurred())

		resp, err := server.Client().Do(req.WithContext(ctx))
		Expect(err).ToNot(HaveOccurred())

		Expect(resp.StatusCode).To(Equal(http.StatusMethodNotAllowed))
	})
})

type stubLogsProvider struct {
	mu             sync.Mutex
	_requests      []*loggregator_v2.EgressBatchRequest
	_batchResponse *loggregator_v2.EnvelopeBatch
	_errorResponse error
}

func newStubLogsProvider() *stubLogsProvider {
	return &stubLogsProvider{}
}

func (s *stubLogsProvider) Stream(ctx context.Context, req *loggregator_v2.EgressBatchRequest) web.Receiver {
	s._requests = append(s._requests, req)

	return func() (*loggregator_v2.EnvelopeBatch, error) {
		return s._batchResponse, s._errorResponse
	}
}

func (s *stubLogsProvider) requests() []*loggregator_v2.EgressBatchRequest {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s._requests
}
