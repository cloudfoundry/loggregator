package proxy_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator/metricemitter/testhelper"
	"code.cloudfoundry.org/loggregator/trafficcontroller/internal/proxy"
	"github.com/gorilla/websocket"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("FirehoseHandler", func() {
	var (
		auth           LogAuthorizer
		adminAuth      AdminAuthorizer
		recorder       *httptest.ResponseRecorder
		connector      *SpyGRPCConnector
		mockSender     *testhelper.SpyMetricClient
		mockHealth     *mockHealth
		logCacheClient *fakeLogCacheClient
	)

	BeforeEach(func() {
		connector = newSpyGRPCConnector(nil)
		logCacheClient = newFakeLogCacheClient()

		adminAuth = AdminAuthorizer{Result: AuthorizerResult{Status: http.StatusOK}}
		auth = LogAuthorizer{Result: AuthorizerResult{Status: http.StatusOK}}

		recorder = httptest.NewRecorder()
		mockSender = testhelper.NewMetricClient()
		mockHealth = newMockHealth()
	})

	It("connects to doppler servers with correct parameters", func() {
		handler := proxy.NewDopplerProxy(
			auth.Authorize,
			adminAuth.Authorize,
			connector,
			"cookieDomain",
			50*time.Millisecond,
			time.Hour,
			mockSender,
			mockHealth,
			newSpyRecentLogsHandler(),
			false,
			logCacheClient,
		)
		req, _ := http.NewRequest("GET", "/firehose/abc-123", nil)
		req.Header.Add("Authorization", "token")

		handler.ServeHTTP(recorder, req)

		Expect(connector.subscriptions.request).To(Equal(&loggregator_v2.EgressBatchRequest{
			ShardId: "abc-123",
			Selectors: []*loggregator_v2.Selector{
				{
					Message: &loggregator_v2.Selector_Counter{
						Counter: &loggregator_v2.CounterSelector{},
					},
				},
				{
					Message: &loggregator_v2.Selector_Gauge{
						Gauge: &loggregator_v2.GaugeSelector{},
					},
				},
				{
					Message: &loggregator_v2.Selector_Log{
						Log: &loggregator_v2.LogSelector{},
					},
				},
				{
					Message: &loggregator_v2.Selector_Timer{
						Timer: &loggregator_v2.TimerSelector{},
					},
				},
			},
			UsePreferredTags: true,
		}))
	})

	It("accepts a query param for filtering logs", func() {
		handler := proxy.NewDopplerProxy(
			auth.Authorize,
			adminAuth.Authorize,
			connector,
			"cookieDomain",
			50*time.Millisecond,
			time.Hour,
			mockSender,
			mockHealth,
			newSpyRecentLogsHandler(),
			false,
			logCacheClient,
		)

		req, err := http.NewRequest("GET", "/firehose/123?filter-type=logs", nil)
		Expect(err).NotTo(HaveOccurred())

		handler.ServeHTTP(recorder, req)

		Expect(connector.subscriptions.request).To(Equal(&loggregator_v2.EgressBatchRequest{
			ShardId: "123",
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

	It("accepts a query param for filtering metrics", func() {
		handler := proxy.NewDopplerProxy(
			auth.Authorize,
			adminAuth.Authorize,
			connector,
			"cookieDomain",
			50*time.Millisecond,
			time.Hour,
			mockSender,
			mockHealth,
			newSpyRecentLogsHandler(),
			false,
			logCacheClient,
		)

		req, err := http.NewRequest("GET", "/firehose/123?filter-type=metrics", nil)
		Expect(err).NotTo(HaveOccurred())

		handler.ServeHTTP(recorder, req)

		Expect(connector.subscriptions.request).To(Equal(&loggregator_v2.EgressBatchRequest{
			ShardId: "123",
			Selectors: []*loggregator_v2.Selector{
				{
					Message: &loggregator_v2.Selector_Counter{
						Counter: &loggregator_v2.CounterSelector{},
					},
				},
				{
					Message: &loggregator_v2.Selector_Gauge{
						Gauge: &loggregator_v2.GaugeSelector{},
					},
				},
				{
					Message: &loggregator_v2.Selector_Timer{
						Timer: &loggregator_v2.TimerSelector{},
					},
				},
			},
			UsePreferredTags: true,
		}))
	})

	It("returns an unauthorized status and sets the WWW-Authenticate header if authorization fails", func() {
		handler := proxy.NewDopplerProxy(
			auth.Authorize,
			adminAuth.Authorize,
			connector,
			"cookieDomain",
			50*time.Millisecond,
			time.Hour,
			mockSender,
			mockHealth,
			newSpyRecentLogsHandler(),
			false,
			logCacheClient,
		)

		adminAuth.Result = AuthorizerResult{Status: http.StatusUnauthorized, ErrorMessage: "Error: Invalid authorization"}

		req, _ := http.NewRequest("GET", "/firehose/abc-123", nil)
		req.Header.Add("Authorization", "token")

		handler.ServeHTTP(recorder, req)

		Expect(adminAuth.TokenParam).To(Equal("token"))

		Expect(recorder.Code).To(Equal(http.StatusUnauthorized))
		Expect(recorder.HeaderMap.Get("WWW-Authenticate")).To(Equal("Basic"))
		Expect(recorder.Body.String()).To(Equal("You are not authorized. Error: Invalid authorization"))
	})

	It("returns a 404 if subscription_id is not provided", func() {
		handler := proxy.NewDopplerProxy(
			auth.Authorize,
			adminAuth.Authorize,
			connector,
			"cookieDomain",
			50*time.Millisecond,
			time.Hour,
			mockSender,
			mockHealth,
			newSpyRecentLogsHandler(),
			false,
			logCacheClient,
		)

		req, _ := http.NewRequest("GET", "/firehose/", nil)
		req.Header.Add("Authorization", "token")

		handler.ServeHTTP(recorder, req)
		Expect(recorder.Code).To(Equal(http.StatusNotFound))
	})

	It("closes the connection to the client on error", func() {
		connector := newSpyGRPCConnector(errors.New("subscribe failed"))
		handler := proxy.NewDopplerProxy(
			auth.Authorize,
			adminAuth.Authorize,
			connector,
			"cookieDomain",
			50*time.Millisecond,
			time.Hour,
			mockSender,
			mockHealth,
			newSpyRecentLogsHandler(),
			false,
			logCacheClient,
		)
		server := httptest.NewServer(handler)
		defer server.CloseClientConnections()

		conn, _, err := websocket.DefaultDialer.Dial(
			wsEndpoint(server, "/firehose/subscription-id"),
			http.Header{"Authorization": []string{"token"}},
		)
		Expect(err).ToNot(HaveOccurred())

		f := func() string {
			_, _, err := conn.ReadMessage()
			return fmt.Sprintf("%s", err)
		}
		Eventually(f).Should(ContainSubstring("websocket: close 1000"))
	})

	It("emits the number of connections as a metric", func() {
		connector := newSpyGRPCConnector(nil)
		handler := proxy.NewDopplerProxy(
			auth.Authorize,
			adminAuth.Authorize,
			connector,
			"cookieDomain",
			50*time.Millisecond,
			time.Hour,
			mockSender,
			mockHealth,
			newSpyRecentLogsHandler(),
			false,
			logCacheClient,
		)
		server := httptest.NewServer(handler)
		defer server.CloseClientConnections()

		conn, _, err := websocket.DefaultDialer.Dial(
			wsEndpoint(server, "/firehose/subscription-id"),
			http.Header{"Authorization": []string{"token"}},
		)
		Expect(err).ToNot(HaveOccurred())

		f := func() float64 {
			return mockSender.GetValue("doppler_proxy.firehoses")
		}
		Eventually(f, 1, "100ms").Should(Equal(1.0))
		conn.Close()
	})
})

func wsEndpoint(server *httptest.Server, path string) string {
	return strings.Replace(server.URL, "http", "ws", 1) + path
}
