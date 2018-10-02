package proxy

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	loggregator "code.cloudfoundry.org/go-loggregator"
	"code.cloudfoundry.org/loggregator/metricemitter"
	"code.cloudfoundry.org/loggregator/plumbing/conversion"
	"github.com/gogo/protobuf/proto"
)

const (
	websocketKeepAliveDuration = 30 * time.Second
	slowConsumerEventTitle     = "Traffic Controller has disconnected slow consumer"
	slowConsumerEventBody      = `Remote Address: %s
X-Forwarded-For: %s
Path: %s

When Loggregator detects a slow connection, that connection is disconnected to prevent back pressure on the system. This may be due to improperly scaled nozzles, or slow user connections to Loggregator`
)

type WebSocketServer struct {
	slowConsumerMetric  *metricemitter.Counter
	slowConsumerTimeout time.Duration
	metricClient        MetricClient
	health              Health
}

func NewWebSocketServer(slowConsumerTimeout time.Duration, m MetricClient, h Health) *WebSocketServer {
	// metric-documentation-v2: (doppler_proxy.slow_consumer) Counter
	// indicating occurrences of slow consumers.
	slowConsumerMetric := m.NewCounter("doppler_proxy.slow_consumer",
		metricemitter.WithVersion(2, 0),
	)

	return &WebSocketServer{
		slowConsumerMetric:  slowConsumerMetric,
		slowConsumerTimeout: slowConsumerTimeout,
		metricClient:        m,
		health:              h,
	}
}

func (s *WebSocketServer) ServeWS(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	recv loggregator.EnvelopeStream,
	egressMetric *metricemitter.Counter,
) {
	data := make(chan []byte)

	handler := NewWebsocketHandler(
		data,
		websocketKeepAliveDuration,
		egressMetric,
	)

	go func() {
		defer close(data)
		timer := time.NewTimer(s.slowConsumerTimeout)
		timer.Stop()
		for {
			resp := recv()
			if resp == nil {
				return
			}

			for _, rv1 := range conversion.ManyToV1(resp) {
				envBytes, err := proto.Marshal(rv1)
				if err != nil {
					continue
				}

				timer.Reset(s.slowConsumerTimeout)
				select {
				case data <- envBytes:
					if !timer.Stop() {
						<-timer.C
					}
				case <-timer.C:
					s.slowConsumerMetric.Increment(1)

					eventBody := fmt.Sprintf(slowConsumerEventBody,
						r.RemoteAddr,
						strings.Join(r.Header["X-Forwarded-For"], ", "),
						r.URL)

					s.metricClient.EmitEvent(
						slowConsumerEventTitle,
						eventBody,
					)
					s.health.Inc("slowConsumerCount")

					log.Printf("Doppler Proxy: Slow Consumer from %s using %s", r.RemoteAddr, r.URL)
					return
				}
			}
		}
	}()

	handler.ServeHTTP(w, r)
}
