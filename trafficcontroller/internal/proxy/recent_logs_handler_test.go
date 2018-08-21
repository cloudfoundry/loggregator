package proxy_test

import (
	"context"
	"errors"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"time"

	logcache "code.cloudfoundry.org/go-log-cache"
	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator/trafficcontroller/internal/proxy"
	"github.com/cloudfoundry/sonde-go/events"
	"github.com/gogo/protobuf/proto"

	"code.cloudfoundry.org/loggregator/metricemitter/testhelper"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Recent Logs Handler", func() {
	var (
		recentLogsHandler *proxy.RecentLogsHandler
		recorder          *httptest.ResponseRecorder
		logCacheClient    *fakeLogCacheClient
	)

	BeforeEach(func() {
		logCacheClient = newFakeLogCacheClient()
		logCacheClient.envelopes = []*loggregator_v2.Envelope{
			buildV2Log("log1"),
			buildV2Log("log2"),
			buildV2Log("log3"),
		}

		recentLogsHandler = proxy.NewRecentLogsHandler(
			logCacheClient,
			200*time.Millisecond,
			testhelper.NewMetricClient(),
		)

		recorder = httptest.NewRecorder()
	})

	It("returns the requested recent logs", func() {
		req, _ := http.NewRequest("GET", "/apps/8de7d390-9044-41ff-ab76-432299923511/recentlogs", nil)
		req.Header.Add("Authorization", "token")
		recentLogs := [][]byte{
			[]byte("log1"),
			[]byte("log2"),
			[]byte("log3"),
		}

		recentLogsHandler.ServeHTTP(recorder, req)

		boundaryRegexp := regexp.MustCompile("boundary=(.*)")
		matches := boundaryRegexp.FindStringSubmatch(recorder.Header().Get("Content-Type"))
		Expect(matches).To(HaveLen(2))
		Expect(matches[1]).NotTo(BeEmpty())
		reader := multipart.NewReader(recorder.Body, matches[1])

		for _, logMessage := range recentLogs {
			part, err := reader.NextPart()
			Expect(err).ToNot(HaveOccurred())

			partBytes, err := ioutil.ReadAll(part)
			Expect(err).ToNot(HaveOccurred())

			var logEnvelope events.Envelope
			err = proto.Unmarshal(partBytes, &logEnvelope)
			Expect(err).ToNot(HaveOccurred())

			Expect(logEnvelope.GetLogMessage().GetMessage()).To(Equal(logMessage))
		}
	})

	It("returns the requested recent logs with limit", func() {
		req, _ := http.NewRequest("GET", "/apps/8de7d390-9044-41ff-ab76-432299923511/recentlogs?limit=2", nil)
		req.Header.Add("Authorization", "token")

		recentLogsHandler.ServeHTTP(recorder, req)

		Expect(logCacheClient.queryParams.Get("limit")).To(Equal("2"))
	})

	It("sets the limit to a 1000 by default", func() {
		req, _ := http.NewRequest("GET", "/apps/8de7d390-9044-41ff-ab76-432299923511/recentlogs", nil)
		req.Header.Add("Authorization", "token")

		recentLogsHandler.ServeHTTP(recorder, req)

		Expect(logCacheClient.queryParams.Get("limit")).To(Equal("1000"))
	})

	It("sets the limit to a 1000 when requested limit is a negative number", func() {
		req, _ := http.NewRequest("GET", "/apps/8de7d390-9044-41ff-ab76-432299923511/recentlogs?limit=-10", nil)
		req.Header.Add("Authorization", "token")

		recentLogsHandler.ServeHTTP(recorder, req)

		Expect(logCacheClient.queryParams.Get("limit")).To(Equal("1000"))
	})

	It("requests envelopes in descending order", func() {
		req, _ := http.NewRequest("GET", "/apps/8de7d390-9044-41ff-ab76-432299923511/recentlogs", nil)
		req.Header.Add("Authorization", "token")

		recentLogsHandler.ServeHTTP(recorder, req)

		Expect(logCacheClient.queryParams.Get("descending")).To(Equal("true"))
	})

	It("returns a helpful error message", func() {
		logCacheClient.err = errors.New("It failed")
		req, _ := http.NewRequest("GET", "/apps/8de7d390-9044-41ff-ab76-432299923511/recentlogs", nil)
		req.Header.Add("Authorization", "token")

		recentLogsHandler.ServeHTTP(recorder, req)
		resp := recorder.Result()
		body, _ := ioutil.ReadAll(resp.Body)

		Expect(resp.StatusCode).To(Equal(http.StatusInternalServerError))
		Expect(string(body)).To(Equal("It failed"))
	})
})

type fakeLogCacheClient struct {
	// Inputs
	sourceID    string
	start       time.Time
	opts        []logcache.ReadOption
	queryParams url.Values

	// Outputs
	envelopes []*loggregator_v2.Envelope
	err       error
}

func newFakeLogCacheClient() *fakeLogCacheClient {
	return &fakeLogCacheClient{
		queryParams: make(map[string][]string),
	}
}

func (f *fakeLogCacheClient) Read(
	ctx context.Context,
	sourceID string,
	start time.Time,
	opts ...logcache.ReadOption,
) ([]*loggregator_v2.Envelope, error) {
	f.sourceID = sourceID
	f.start = start
	f.opts = opts

	for _, o := range opts {
		o(nil, f.queryParams)
	}

	return f.envelopes, f.err
}

func buildV2Log(msg string) *loggregator_v2.Envelope {
	return &loggregator_v2.Envelope{
		Message: &loggregator_v2.Envelope_Log{
			Log: &loggregator_v2.Log{
				Payload: []byte(msg),
			},
		},
	}
}

func buildV1LogBytes(msg string) []byte {
	envelopeBytes, _ := (&events.Envelope{
		Origin:    proto.String(""),
		EventType: events.Envelope_LogMessage.Enum(),
		LogMessage: &events.LogMessage{
			Message: []byte(msg),
		},
	}).Marshal()
	return envelopeBytes
}
