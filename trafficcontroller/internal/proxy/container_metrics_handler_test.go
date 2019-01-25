package proxy_test

import (
	"context"
	"errors"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"time"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator/metricemitter/testhelper"
	"code.cloudfoundry.org/loggregator/trafficcontroller/internal/proxy"

	"github.com/cloudfoundry/sonde-go/events"
	"github.com/gorilla/mux"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ContainerMetricsHandler", func() {
	var (
		logCacheClient  *fakeLogCacheClient
		spyMetricClient *testhelper.SpyMetricClient

		server *httptest.Server
	)

	BeforeEach(func() {
		logCacheClient = newFakeLogCacheClient()
		logCacheClient.envelopes = []*loggregator_v2.Envelope{
			buildV2Gauge("some-id", "1", 1, 1),
			buildV2Gauge("some-id", "1", 2, 2),
			buildV2Gauge("some-id", "2", 3, 3),
			buildV2Log("log1"),
			buildV2Log("log2"),
			buildV2Log("log3"),
		}

		spyMetricClient = testhelper.NewMetricClient()
		mux := mux.NewRouter()
		mux.Handle("/apps/{appID}/containermetrics", proxy.NewContainerMetricsHandler(
			logCacheClient,
			time.Hour,
			spyMetricClient,
		))

		server = httptest.NewServer(mux)
	})

	It("returns ContainerMetrics via multipart message", func() {
		resp, err := http.Get(server.URL + "/apps/some-id/containermetrics")
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		// Ensure context has timeout
		_, ok := logCacheClient.ctx.Deadline()
		Expect(ok).To(BeTrue())
		Expect(logCacheClient.ctx).ToNot(BeNil())

		Expect(logCacheClient.sourceID).To(Equal("some-id"))
		Expect(logCacheClient.start.UnixNano()).To(BeNumerically("~", time.Now().Add(-time.Minute).UnixNano(), time.Second))
		Expect(logCacheClient.queryParams).To(HaveKeyWithValue("envelope_types", []string{"GAUGE"}))

		es := multipartToEnvelopes(resp.Header, resp.Body)

		sort.Sort(envelopes(es))

		Expect(es[0].GetTimestamp()).To(Equal(int64(2)))
		Expect(es[0].ContainerMetric.GetCpuPercentage()).To(Equal(float64(2)))

		Expect(es[1].GetTimestamp()).To(Equal(int64(3)))
		Expect(es[1].ContainerMetric.GetCpuPercentage()).To(Equal(float64(3)))
	})

	It("gives a 500 when LogCache returns an error", func() {
		logCacheClient.err <- errors.New("some-error")
		resp, err := http.Get(server.URL + "/apps/some-id/containermetrics")
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusInternalServerError))
	})

	It("uses the context from the HTTP request", func() {
		logCacheClient.block = true
		req, err := http.NewRequest("GET", server.URL+"/apps/some-id/containermetrics", nil)
		Expect(err).ToNot(HaveOccurred())
		ctx, _ := context.WithTimeout(context.Background(), 250*time.Millisecond)
		req = req.WithContext(ctx)

		go func() {
			http.DefaultClient.Do(req)
		}()

		Eventually(func() error {
			logCacheClient.mu.Lock()
			defer logCacheClient.mu.Unlock()
			if logCacheClient.ctx != nil {
				return logCacheClient.ctx.Err()
			}
			return nil
		}).ShouldNot(BeNil())
	})

	// TODO: This should not be something that exists beyond CloudFoundry's
	// hesitation around LogCache. Once LogCache is garunteed, this feature
	// should be removed.
	It("gives empty results if LogCacheClient is nil", func() {
		p := proxy.NewContainerMetricsHandler(
			nil,
			time.Hour,
			spyMetricClient,
		)

		recorder := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/does-not-matter", nil)

		p.ServeHTTP(recorder, req)

		es := multipartToEnvelopes(recorder.Header(), recorder.Body)

		Expect(recorder.Code).To(Equal(http.StatusOK))
		Expect(es).To(HaveLen(1))
	})
})

type envelopes []*events.Envelope

func (e envelopes) Len() int {
	return len(e)
}

func (e envelopes) Less(i, j int) bool {
	return e[i].GetTimestamp() < e[j].GetTimestamp()
}

func (e envelopes) Swap(i, j int) {
	tmp := e[i]
	e[i] = e[j]
	e[j] = tmp
}

func multipartToEnvelopes(header http.Header, body io.Reader) []*events.Envelope {
	boundaryRegexp := regexp.MustCompile("boundary=(.*)")
	matches := boundaryRegexp.FindStringSubmatch(header.Get("Content-Type"))
	ExpectWithOffset(1, matches).To(HaveLen(2))
	ExpectWithOffset(1, matches[1]).NotTo(BeEmpty())
	reader := multipart.NewReader(body, matches[1])

	var es []*events.Envelope
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}

		ExpectWithOffset(1, err).ToNot(HaveOccurred())
		partBytes, err := ioutil.ReadAll(part)
		ExpectWithOffset(1, err).ToNot(HaveOccurred())

		var e events.Envelope
		ExpectWithOffset(1, e.Unmarshal(partBytes)).To(Succeed())
		es = append(es, &e)
	}
	return es
}
