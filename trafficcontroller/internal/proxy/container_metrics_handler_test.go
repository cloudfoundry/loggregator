package proxy_test

import (
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"time"

	"code.cloudfoundry.org/loggregator/metricemitter/testhelper"
	"code.cloudfoundry.org/loggregator/trafficcontroller/internal/proxy"

	"github.com/cloudfoundry/sonde-go/events"
	"github.com/gorilla/mux"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ContainerMetricsHandler", func() {
	var (
		spyGrpcConnector *SpyGRPCConnector
		spyMetricClient  *testhelper.SpyMetricClient

		server *httptest.Server
	)

	BeforeEach(func() {
		spyGrpcConnector = newSpyGRPCConnector(nil)
		spyMetricClient = testhelper.NewMetricClient()
		mux := mux.NewRouter()
		mux.Handle("/apps/{appID}/containermetrics", proxy.NewContainerMetricsHandler(
			spyGrpcConnector,
			time.Millisecond,
			spyMetricClient,
		))

		server = httptest.NewServer(mux)
	})

	It("returns ContainerMetrics via multipart message", func() {
		resp, err := http.Get(server.URL + "/apps/some-id/containermetrics")
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		// Ensure context has timeout
		_, ok := spyGrpcConnector.containerMetrics.ctx.Deadline()
		Expect(ok).To(BeTrue())
		Expect(spyGrpcConnector.containerMetrics.ctx).ToNot(BeNil())

		Expect(spyGrpcConnector.containerMetrics.appID).To(Equal("some-id"))

		// Multi-part stuff
		boundaryRegexp := regexp.MustCompile("boundary=(.*)")
		matches := boundaryRegexp.FindStringSubmatch(resp.Header.Get("Content-Type"))
		Expect(matches).To(HaveLen(2))
		Expect(matches[1]).NotTo(BeEmpty())
		reader := multipart.NewReader(resp.Body, matches[1])

		var es []*events.Envelope
		for i := 0; i < 2; i++ {
			part, err := reader.NextPart()
			Expect(err).ToNot(HaveOccurred())
			partBytes, err := ioutil.ReadAll(part)
			Expect(err).ToNot(HaveOccurred())

			var e events.Envelope
			Expect(e.Unmarshal(partBytes)).To(Succeed())
			es = append(es, &e)
		}

		sort.Sort(envelopes(es))

		Expect(es[0].GetTimestamp()).To(Equal(int64(11)))
		Expect(es[0].ContainerMetric.GetCpuPercentage()).To(Equal(float64(2)))

		Expect(es[1].GetTimestamp()).To(Equal(int64(12)))
		Expect(es[1].ContainerMetric.GetCpuPercentage()).To(Equal(float64(3)))
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
