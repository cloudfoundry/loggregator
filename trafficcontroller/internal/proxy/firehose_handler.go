package proxy

import (
	"context"
	"net/http"
	"sync/atomic"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator/metricemitter"

	"github.com/gorilla/mux"
)

var (
	logSelectors = []*loggregator_v2.Selector{
		{
			Message: &loggregator_v2.Selector_Log{
				Log: &loggregator_v2.LogSelector{},
			},
		},
	}

	metricSelectors = []*loggregator_v2.Selector{
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
	}

	allSelectors = []*loggregator_v2.Selector{
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
	}
)

type FirehoseHandler struct {
	server               *WebSocketServer
	grpcConn             GrpcConnector
	counter              int64
	egressFirehoseMetric *metricemitter.Counter
}

func NewFirehoseHandler(grpcConn GrpcConnector, w *WebSocketServer, m MetricClient) *FirehoseHandler {
	// metric-documentation-v2: (egress) Number of envelopes egressed via the
	// firehose.
	egressFirehoseMetric := m.NewCounter("egress",
		metricemitter.WithVersion(2, 0),
		metricemitter.WithTags(
			map[string]string{"endpoint": "firehose"},
		),
	)

	return &FirehoseHandler{
		grpcConn:             grpcConn,
		server:               w,
		egressFirehoseMetric: egressFirehoseMetric,
	}
}

func (h *FirehoseHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt64(&h.counter, 1)
	defer atomic.AddInt64(&h.counter, -1)

	subID := mux.Vars(r)["subID"]

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var selectors []*loggregator_v2.Selector
	switch r.URL.Query().Get("filter-type") {
	case "logs":
		selectors = logSelectors
	case "metrics":
		selectors = metricSelectors
	default:
		selectors = allSelectors
	}

	stream := h.grpcConn.Stream(ctx, &loggregator_v2.EgressBatchRequest{
		ShardId:          subID,
		Selectors:        selectors,
		UsePreferredTags: true,
	})

	h.server.ServeWS(ctx, w, r, stream, h.egressFirehoseMetric)
}

func (h *FirehoseHandler) Count() int64 {
	return atomic.LoadInt64(&h.counter)
}
