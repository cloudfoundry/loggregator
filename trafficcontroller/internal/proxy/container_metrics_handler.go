package proxy

import (
	"context"
	"log"
	"net/http"
	"time"

	logcache "code.cloudfoundry.org/go-log-cache"
	"code.cloudfoundry.org/go-log-cache/rpc/logcache_v1"
	"code.cloudfoundry.org/loggregator/metricemitter"
	"code.cloudfoundry.org/loggregator/plumbing/conversion"

	"github.com/cloudfoundry/sonde-go/events"
	"github.com/gogo/protobuf/proto"
	"github.com/gorilla/mux"
)

type ContainerMetricsHandler struct {
	logCacheClient LogCacheClient
	timeout        time.Duration
	latencyMetric  *metricemitter.Gauge
}

func NewContainerMetricsHandler(
	logCacheClient LogCacheClient,
	t time.Duration,
	m MetricClient,
) *ContainerMetricsHandler {
	// metric-documentation-v2: (doppler_proxy.container_metrics_latency)
	// Measures amount of time to serve the request for container metrics
	latencyMetric := m.NewGauge("doppler_proxy.container_metrics_latency", "ms",
		metricemitter.WithVersion(2, 0),
	)

	return &ContainerMetricsHandler{
		logCacheClient: logCacheClient,
		timeout:        t,
		latencyMetric:  latencyMetric,
	}
}

func (h *ContainerMetricsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	defer func() {
		elapsedMillisecond := float64(time.Since(startTime)) / float64(time.Millisecond)

		h.latencyMetric.Set(elapsedMillisecond)
	}()

	appID := mux.Vars(r)["appID"]

	if h.logCacheClient == nil {
		serveMultiPartResponse(w, [][]byte{emptyContainerMetric(appID)})
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	ctx, _ = context.WithDeadline(ctx, time.Now().Add(h.timeout))
	defer cancel()

	es, err := h.logCacheClient.Read(ctx, appID, time.Now().Add(-time.Minute),
		logcache.WithEnvelopeTypes(logcache_v1.EnvelopeType_GAUGE),
	)
	if err != nil {
		log.Printf("LogCache request failed: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var v1Es []*events.Envelope
	for _, e := range es {
		v1Es = append(v1Es, conversion.ToV1(e)...)
	}

	resp := deDupe(v1Es)
	serveMultiPartResponse(w, resp)
}

func deDupe(input []*events.Envelope) [][]byte {
	messages := make(map[int32]*events.Envelope)

	for _, envelope := range input {
		cm := envelope.GetContainerMetric()
		if cm == nil {
			continue
		}

		oldEnvelope, ok := messages[cm.GetInstanceIndex()]
		if !ok || oldEnvelope.GetTimestamp() < envelope.GetTimestamp() {
			messages[cm.GetInstanceIndex()] = envelope
		}
	}

	output := make([][]byte, 0, len(messages))

	for _, envelope := range messages {
		bytes, _ := proto.Marshal(envelope)
		output = append(output, bytes)
	}
	return output
}

func emptyContainerMetric(appID string) []byte {
	data, err := proto.Marshal(&events.Envelope{
		Origin:    proto.String("loggr"),
		Timestamp: proto.Int64(time.Now().UnixNano()),
		EventType: events.Envelope_ContainerMetric.Enum(),
		ContainerMetric: &events.ContainerMetric{
			ApplicationId:    proto.String(appID),
			InstanceIndex:    proto.Int32(0),
			CpuPercentage:    proto.Float64(0),
			MemoryBytes:      proto.Uint64(0),
			DiskBytes:        proto.Uint64(0),
			MemoryBytesQuota: proto.Uint64(0),
			DiskBytesQuota:   proto.Uint64(0),
		},
	})
	if err != nil {
		log.Panicf("marshalling failure: %s", err)
	}

	return data
}
