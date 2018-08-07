package v2

import (
	"io"
	"log"
	"time"

	gendiode "code.cloudfoundry.org/go-diodes"
	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator/diodes"
	"code.cloudfoundry.org/loggregator/metricemitter"
	"code.cloudfoundry.org/loggregator/plumbing/batching"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

// Subscriber registers stream DataSetters to accept reads.
type Subscriber interface {
	Subscribe(req *loggregator_v2.EgressBatchRequest, setter DataSetter) (unsubscribe func())
}

// DataSetter accepts writes of v2.Envelopes
// TODO: This could be a named function. This will be a performance bump.
type DataSetter interface {
	Set(*loggregator_v2.Envelope)
}

// EgressServer implements the loggregator_v2.EgressServer interface.
type EgressServer struct {
	subscriber          Subscriber
	egressMetric        *metricemitter.Counter
	subscriptionsMetric *metricemitter.Gauge
	health              HealthRegistrar
	forceFlushInterval  time.Duration
	batchInterval       time.Duration
	batchSize           uint
}

// NewEgressServer is the constructor for EgressServer.
func NewEgressServer(
	s Subscriber,
	m MetricClient,
	subscriptionsMetric *metricemitter.Gauge,
	h HealthRegistrar,
	batchInterval time.Duration,
	batchSize uint,
	forceFlushInterval time.Duration,
) *EgressServer {
	// metric-documentation-v2: (loggregator.doppler.egress) Number of
	// envelopes read from a diode to be sent to subscriptions.
	egressMetric := m.NewCounter("egress",
		metricemitter.WithVersion(2, 0),
	)
	return &EgressServer{
		subscriber:          s,
		egressMetric:        egressMetric,
		subscriptionsMetric: subscriptionsMetric,
		health:              h,
		batchInterval:       batchInterval,
		batchSize:           batchSize,
		forceFlushInterval:  forceFlushInterval,
	}
}

// Alert logs dropped message counts to stderr.
func (*EgressServer) Alert(missed int) {
	log.Printf("Dropped %d envelopes (v2 Egress Server)", missed)
}

// Receiver implements loggregator_v2.EgressServer.
func (s *EgressServer) Receiver(
	req *loggregator_v2.EgressRequest,
	sender loggregator_v2.Egress_ReceiverServer,
) error {
	return grpc.Errorf(codes.Unimplemented, "use BatchedReceiver instead")
}

// BatchedReceiver implements loggregator_v2.EgressServer.
func (s *EgressServer) BatchedReceiver(
	req *loggregator_v2.EgressBatchRequest,
	sender loggregator_v2.Egress_BatchedReceiverServer,
) error {
	s.subscriptionsMetric.Increment(1.0)
	defer s.subscriptionsMetric.Decrement(1.0)
	s.health.Inc("subscriptionCount")
	defer s.health.Dec("subscriptionCount")

	d := diodes.NewOneToOneEnvelopeV2(1000, s,
		gendiode.WithWaiterContext(sender.Context()),
	)

	cancel := s.subscriber.Subscribe(req, d)
	defer cancel()

	errStream := make(chan error, 1)
	batcher := batching.NewV2EnvelopeBatcher(
		int(s.batchSize),
		s.batchInterval,
		&batchWriter{
			sender:       sender,
			errStream:    errStream,
			egressMetric: s.egressMetric,
		},
	)

	dw := newDiodeWrapper(d)

	t := time.NewTimer(s.forceFlushInterval)
	for {
		if !t.Stop() {
			<-t.C
		}
		t.Reset(s.forceFlushInterval)

		select {
		case <-sender.Context().Done():
			return sender.Context().Err()
		case err := <-errStream:
			return err
		case e, ok := <-dw.next():
			if !ok {
				batcher.ForcedFlush()
				return io.EOF
			}
			batcher.Write(e)
		case <-t.C:
			batcher.ForcedFlush()
		}
	}
}

type batchWriter struct {
	sender       loggregator_v2.Egress_BatchedReceiverServer
	errStream    chan<- error
	egressMetric *metricemitter.Counter
}

// Write adds an entry to the batch. If the batch conditions are met, the
// batch is flushed.
func (b *batchWriter) Write(batch []*loggregator_v2.Envelope) {
	err := b.sender.Send(&loggregator_v2.EnvelopeBatch{
		Batch: batch,
	})
	if err != nil {
		b.errStream <- err
		return
	}
	b.egressMetric.Increment(uint64(len(batch)))
}

type diodeWrapper struct {
	d    *diodes.OneToOneEnvelopeV2
	data chan *loggregator_v2.Envelope
}

func newDiodeWrapper(d *diodes.OneToOneEnvelopeV2) diodeWrapper {
	dw := diodeWrapper{
		d:    d,
		data: make(chan *loggregator_v2.Envelope, 100),
	}

	go func() {
		for {
			e := dw.d.Next()
			if e == nil {
				close(dw.data)
				return
			}
			dw.data <- e
		}
	}()

	return dw
}

func (dw diodeWrapper) Set(e *loggregator_v2.Envelope) {
	dw.d.Set(e)
}

func (dw diodeWrapper) next() chan *loggregator_v2.Envelope {
	return dw.data
}
