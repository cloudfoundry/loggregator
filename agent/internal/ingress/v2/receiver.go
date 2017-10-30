package v2

import (
	"log"

	"code.cloudfoundry.org/loggregator/metricemitter"
	v2 "code.cloudfoundry.org/loggregator/plumbing/v2"
	"golang.org/x/net/context"
)

type DataSetter interface {
	Set(e *v2.Envelope)
}

// MetricClient creates new CounterMetrics to be emitted periodically.
type MetricClient interface {
	NewCounter(name string, opts ...metricemitter.MetricOption) *metricemitter.Counter
}

type Receiver struct {
	dataSetter    DataSetter
	ingressMetric *metricemitter.Counter
}

func NewReceiver(dataSetter DataSetter, metricClient MetricClient) *Receiver {
	// metric-documentation-v2: (loggregator.metron.ingress) The number of
	// received messages over Metrons V2 gRPC API.
	ingressMetric := metricClient.NewCounter("ingress",
		metricemitter.WithVersion(2, 0),
	)

	return &Receiver{
		dataSetter:    dataSetter,
		ingressMetric: ingressMetric,
	}
}

func (s *Receiver) Sender(sender v2.Ingress_SenderServer) error {
	for {
		e, err := sender.Recv()
		if err != nil {
			log.Printf("Failed to receive data: %s", err)
			return err
		}

		s.dataSetter.Set(e)
		s.ingressMetric.Increment(1)
	}

	return nil
}

func (s *Receiver) BatchSender(sender v2.Ingress_BatchSenderServer) error {
	for {
		envelopes, err := sender.Recv()
		if err != nil {
			log.Printf("Failed to receive data: %s", err)
			return err
		}

		for _, e := range envelopes.Batch {
			s.dataSetter.Set(e)
		}
		s.ingressMetric.Increment(uint64(len(envelopes.Batch)))
	}

	return nil
}

func (s *Receiver) Send(_ context.Context, b *v2.EnvelopeBatch) (*v2.SendResponse, error) {
	for _, e := range b.Batch {
		s.dataSetter.Set(e)
	}

	s.ingressMetric.Increment(uint64(len(b.Batch)))

	return &v2.SendResponse{}, nil
}
