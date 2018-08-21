package proxy

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	logcache "code.cloudfoundry.org/go-log-cache"
	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator/metricemitter"
	"code.cloudfoundry.org/loggregator/plumbing/conversion"
	"github.com/gogo/protobuf/proto"
	"github.com/gorilla/mux"
)

type LogCacheClient interface {
	Read(
		ctx context.Context,
		sourceID string,
		start time.Time,
		opts ...logcache.ReadOption,
	) ([]*loggregator_v2.Envelope, error)
}

type RecentLogsHandler struct {
	recentLogProvider LogCacheClient
	timeout           time.Duration
	latencyMetric     *metricemitter.Gauge
}

func NewRecentLogsHandler(
	recentLogProvider LogCacheClient,
	t time.Duration,
	m MetricClient,
) *RecentLogsHandler {
	// metric-documentation-v2: (doppler_proxy.recent_logs_latency) Measures
	// amount of time to serve the request for recent logs
	latencyMetric := m.NewGauge("doppler_proxy.recent_logs_latency", "ms",
		metricemitter.WithVersion(2, 0),
	)

	return &RecentLogsHandler{
		recentLogProvider: recentLogProvider,
		timeout:           t,
		latencyMetric:     latencyMetric,
	}
}

func (h *RecentLogsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	defer func() {
		elapsedMillisecond := float64(time.Since(startTime)) / float64(time.Millisecond)
		h.latencyMetric.Set(elapsedMillisecond)
	}()

	appID := mux.Vars(r)["appID"]

	ctx, cancel := context.WithCancel(context.Background())
	ctx, _ = context.WithDeadline(ctx, time.Now().Add(h.timeout))
	defer cancel()

	limit, ok := limitFrom(r)
	if !ok {
		limit = 1000
	}

	envelopes, err := h.recentLogProvider.Read(
		ctx,
		appID,
		time.Unix(0, 0),
		logcache.WithLimit(limit),
		logcache.WithDescending(),
	)

	if err != nil {
		log.Printf("error communicating with log cache: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	var resp [][]byte
	for _, v2e := range envelopes {
		for _, v1e := range conversion.ToV1(v2e) {
			v1bytes, err := proto.Marshal(v1e)
			if err != nil {
				log.Printf("error marshalling v1 envelope for recent log response: %s", err)
				continue
			}
			resp = append(resp, v1bytes)
		}
	}
	serveMultiPartResponse(w, resp)
}

func limitFrom(r *http.Request) (int, bool) {
	query := r.URL.Query()
	values, ok := query["limit"]
	if !ok {
		return 0, false
	}

	value, err := strconv.Atoi(values[0])
	if err != nil || value < 0 {
		return 0, false
	}

	return value, true
}
