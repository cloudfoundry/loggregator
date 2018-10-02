package proxy

import (
	"context"
	"net/http"
	"sync/atomic"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator/metricemitter"

	"github.com/gorilla/mux"
)

var includedGuageNames = []string{
	"cpuPercentage",
	"memoryBytes",
	"diskBytes",
	"memoryBytesQuota",
	"diskBytesQuota",
}

type StreamHandler struct {
	server       *WebSocketServer
	grpcConn     GrpcConnector
	counter      int64
	egressMetric *metricemitter.Counter
}

func NewStreamHandler(grpcConn GrpcConnector, w *WebSocketServer, m MetricClient) *StreamHandler {
	// metric-documentation-v2: (egress) Number of envelopes egressed via
	// an app stream.
	egressMetric := m.NewCounter("egress",
		metricemitter.WithVersion(2, 0),
		metricemitter.WithTags(
			map[string]string{"endpoint": "stream"},
		),
	)

	return &StreamHandler{
		grpcConn:     grpcConn,
		server:       w,
		egressMetric: egressMetric,
	}
}

func (h *StreamHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt64(&h.counter, 1)
	defer atomic.AddInt64(&h.counter, -1)

	appID := mux.Vars(r)["appID"]

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := h.grpcConn.Stream(ctx, &loggregator_v2.EgressBatchRequest{
		Selectors: []*loggregator_v2.Selector{
			{
				SourceId: appID,
				Message: &loggregator_v2.Selector_Log{
					Log: &loggregator_v2.LogSelector{},
				},
			},
			{
				SourceId: appID,
				Message: &loggregator_v2.Selector_Gauge{
					Gauge: &loggregator_v2.GaugeSelector{
						Names: includedGuageNames,
					},
				},
			},
		},
		UsePreferredTags: true,
	})

	h.server.ServeWS(ctx, w, r, stream, h.egressMetric)
}

func (h *StreamHandler) Count() int64 {
	return atomic.LoadInt64(&h.counter)
}
