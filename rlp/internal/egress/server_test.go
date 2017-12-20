package egress_test

import (
	"errors"
	"io"
	"sync"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"

	v2 "code.cloudfoundry.org/loggregator/plumbing/v2"
	"code.cloudfoundry.org/loggregator/rlp/internal/egress"

	"code.cloudfoundry.org/loggregator/metricemitter/testhelper"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Server", func() {
	Describe("Receiver()", func() {
		It("errors when the sender cannot send the envelope", func() {
			receiverServer := newSpyReceiverServer(errors.New("Oh No!"))
			receiver := newSpyReceiver(1)
			server := egress.NewServer(
				receiver,
				testhelper.NewMetricClient(),
				newSpyHealthRegistrar(),
				context.TODO(),
				1,
				time.Nanosecond,
			)

			err := server.Receiver(&v2.EgressRequest{}, receiverServer)

			Expect(err).To(Equal(io.ErrUnexpectedEOF))
		})

		It("streams data when there are envelopes", func() {
			receiverServer := newSpyReceiverServer(nil)
			receiver := newSpyReceiver(10)
			server := egress.NewServer(
				receiver,
				testhelper.NewMetricClient(),
				newSpyHealthRegistrar(),
				context.TODO(),
				1,
				time.Nanosecond,
			)

			err := server.Receiver(&v2.EgressRequest{}, receiverServer)
			Expect(err).ToNot(HaveOccurred())
			Eventually(receiverServer.envelopes).Should(HaveLen(10))
		})

		It("forwards the request as an egress batch request", func() {
			receiverServer := newSpyReceiverServer(nil)
			receiver := newSpyReceiver(10)
			server := egress.NewServer(
				receiver,
				testhelper.NewMetricClient(),
				newSpyHealthRegistrar(),
				context.TODO(),
				1,
				time.Nanosecond,
			)

			egressReq := &v2.EgressRequest{
				ShardId: "a-shard-id",
				Selectors: []*v2.Selector{
					{SourceId: "a-source-id"},
				},
				UsePreferredTags: true,
			}
			err := server.Receiver(egressReq, receiverServer)
			Expect(err).ToNot(HaveOccurred())

			var egressBatchReq *v2.EgressBatchRequest
			Eventually(receiver.requests).Should(Receive(&egressBatchReq))
			Expect(egressBatchReq.GetShardId()).To(Equal(egressReq.GetShardId()))
			Expect(egressBatchReq.GetSelectors()).To(Equal(egressReq.GetSelectors()))
			Expect(egressBatchReq.GetUsePreferredTags()).To(Equal(egressReq.GetUsePreferredTags()))
		})

		It("ignores the legacy selector if both old and new selectors are used", func() {
			receiverServer := newSpyReceiverServer(nil)
			receiver := newSpyReceiver(10)
			server := egress.NewServer(
				receiver,
				testhelper.NewMetricClient(),
				newSpyHealthRegistrar(),
				context.TODO(),
				1,
				time.Nanosecond,
			)

			err := server.Receiver(&v2.EgressRequest{
				LegacySelector: &v2.Selector{
					SourceId: "source-id",
				},
				Selectors: []*v2.Selector{
					{SourceId: "source-id"},
					{SourceId: "other-source-id"},
				},
			}, receiverServer)
			Expect(err).ToNot(HaveOccurred())

			var req *v2.EgressBatchRequest
			Expect(receiver.requests).Should(Receive(&req))
			Expect(req.Selectors).Should(HaveLen(2))

			Expect(req.LegacySelector).To(BeNil())
		})

		It("upgrades LegacySelector to Selector if no Selectors are present", func() {
			receiverServer := newSpyReceiverServer(nil)
			receiver := newSpyReceiver(10)
			server := egress.NewServer(
				receiver,
				testhelper.NewMetricClient(),
				newSpyHealthRegistrar(),
				context.TODO(),
				1,
				time.Nanosecond,
			)

			err := server.Receiver(&v2.EgressRequest{
				LegacySelector: &v2.Selector{
					SourceId: "legacy-source-id",
				},
			}, receiverServer)
			Expect(err).ToNot(HaveOccurred())

			var req *v2.EgressBatchRequest
			Expect(receiver.requests).Should(Receive(&req))
			Expect(req.Selectors).Should(HaveLen(1))

			Expect(req.LegacySelector).To(BeNil())
			Expect(req.Selectors[0].SourceId).To(Equal("legacy-source-id"))
		})

		It("closes the receiver when the context is canceled", func() {
			receiverServer := newSpyReceiverServer(nil)
			receiver := newSpyReceiver(1000000000)
			ctx, cancel := context.WithCancel(context.TODO())
			server := egress.NewServer(
				receiver,
				testhelper.NewMetricClient(),
				newSpyHealthRegistrar(),
				ctx,
				1,
				time.Nanosecond,
			)

			go func() {
				err := server.Receiver(&v2.EgressRequest{}, receiverServer)
				Expect(err).ToNot(HaveOccurred())
			}()

			cancel()

			var rxCtx context.Context
			Eventually(receiver.ctx).Should(Receive(&rxCtx))
			Eventually(rxCtx.Done).Should(BeClosed())
		})

		It("cancels the context when Receiver exits", func() {
			receiverServer := newSpyReceiverServer(errors.New("Oh no!"))
			receiver := newSpyReceiver(100000000)
			server := egress.NewServer(
				receiver,
				testhelper.NewMetricClient(),
				newSpyHealthRegistrar(),
				context.TODO(),
				1,
				time.Nanosecond,
			)

			go server.Receiver(&v2.EgressRequest{}, receiverServer)

			var ctx context.Context
			Eventually(receiver.ctx).Should(Receive(&ctx))
			Eventually(ctx.Done()).Should(BeClosed())
		})

		Describe("Preferred Tags option", func() {
			var (
				receiverServer *spyReceiverServer
				receiver       *spyReceiver
				server         *egress.Server
			)

			BeforeEach(func() {
				receiverServer = newSpyReceiverServer(nil)
				receiver = newSpyReceiver(10)
				receiver.envelope = &v2.Envelope{
					Tags: map[string]string{
						"a": "value-a",
					},
					DeprecatedTags: map[string]*v2.Value{
						"b": {
							Data: &v2.Value_Decimal{
								Decimal: 0.8,
							},
						},
						"c": {
							Data: &v2.Value_Integer{
								Integer: 18,
							},
						},
						"d": {
							Data: &v2.Value_Text{
								Text: "value-d",
							},
						},
					},
				}
				server = egress.NewServer(
					receiver,
					testhelper.NewMetricClient(),
					newSpyHealthRegistrar(),
					context.TODO(),
					1,
					time.Nanosecond,
				)
			})

			It("sends deprecated tags", func() {
				err := server.Receiver(&v2.EgressRequest{
					Selectors:        []*v2.Selector{},
					UsePreferredTags: false,
				}, receiverServer)
				Expect(err).ToNot(HaveOccurred())

				var e *v2.Envelope
				Eventually(receiverServer.envelopes).Should(Receive(&e))
				Expect(e.GetTags()).To(BeEmpty())
				Expect(e.GetDeprecatedTags()).To(HaveLen(4))

				tags := e.GetDeprecatedTags()
				Expect(tags["a"].GetText()).To(Equal("value-a"))
				Expect(tags["b"].GetDecimal()).To(Equal(0.8))
				Expect(tags["c"].GetInteger()).To(Equal(int64(18)))
				Expect(tags["d"].GetText()).To(Equal("value-d"))
			})

			It("sends preferred tags", func() {
				err := server.Receiver(&v2.EgressRequest{
					Selectors:        []*v2.Selector{},
					UsePreferredTags: true,
				}, receiverServer)
				Expect(err).ToNot(HaveOccurred())

				var e *v2.Envelope
				Eventually(receiverServer.envelopes).Should(Receive(&e))
				Expect(e.GetDeprecatedTags()).To(BeEmpty())
				Expect(e.GetTags()).To(HaveLen(4))

				tags := e.GetTags()
				Expect(tags["a"]).To(Equal("value-a"))
				Expect(tags["b"]).To(Equal("0.8"))
				Expect(tags["c"]).To(Equal("18"))
				Expect(tags["d"]).To(Equal("value-d"))
			})
		})

		Describe("Metrics", func() {
			It("emits 'egress' metric for each envelope", func() {
				metricClient := testhelper.NewMetricClient()
				receiverServer := newSpyReceiverServer(nil)
				receiver := newSpyReceiver(10)
				server := egress.NewServer(
					receiver,
					metricClient,
					newSpyHealthRegistrar(),
					context.TODO(),
					1,
					time.Nanosecond,
				)

				err := server.Receiver(&v2.EgressRequest{}, receiverServer)

				Expect(err).ToNot(HaveOccurred())
				Eventually(func() uint64 {
					return metricClient.GetDelta("egress")
				}).Should(BeNumerically("==", 10))
			})

			It("emits 'dropped' metric for each envelope", func() {
				metricClient := testhelper.NewMetricClient()
				receiverServer := newSpyReceiverServer(nil)
				receiverServer.wait = make(chan struct{})
				defer receiverServer.stopWait()

				receiver := newSpyReceiver(1000000)
				server := egress.NewServer(
					receiver,
					metricClient,
					newSpyHealthRegistrar(),
					context.TODO(),
					1,
					time.Nanosecond,
				)

				go server.Receiver(&v2.EgressRequest{}, receiverServer)

				Eventually(func() uint64 {
					return metricClient.GetDelta("dropped")
				}, 3).Should(BeNumerically(">", 100))
			})
		})

		Describe("health monitoring", func() {
			It("increments and decrements subscription count", func() {
				receiverServer := newSpyReceiverServer(nil)
				receiver := newSpyReceiver(1000000000)

				health := newSpyHealthRegistrar()
				server := egress.NewServer(
					receiver,
					testhelper.NewMetricClient(),
					health,
					context.TODO(),
					1,
					time.Nanosecond,
				)
				go server.Receiver(&v2.EgressRequest{}, receiverServer)

				Eventually(func() float64 {
					return health.Get("subscriptionCount")
				}).Should(Equal(1.0))

				receiver.stop()

				Eventually(func() float64 {
					return health.Get("subscriptionCount")
				}).Should(Equal(0.0))
			})
		})
	})

	Describe("BatchedReceiver()", func() {
		It("errors when the sender cannot send the envelope", func() {
			receiverServer := newSpyBatchedReceiverServer(errors.New("Oh No!"))
			server := egress.NewServer(
				&stubReceiver{},
				testhelper.NewMetricClient(),
				newSpyHealthRegistrar(),
				context.TODO(),
				1,
				time.Nanosecond,
			)

			err := server.BatchedReceiver(&v2.EgressBatchRequest{}, receiverServer)

			Expect(err).To(Equal(io.ErrUnexpectedEOF))
		})

		It("streams data when there are envelopes", func() {
			receiverServer := newSpyBatchedReceiverServer(nil)
			server := egress.NewServer(
				newSpyReceiver(10),
				testhelper.NewMetricClient(),
				newSpyHealthRegistrar(),
				context.TODO(),
				10,
				time.Nanosecond,
			)

			err := server.BatchedReceiver(&v2.EgressBatchRequest{}, receiverServer)

			Expect(err).ToNot(HaveOccurred())
			Eventually(receiverServer.envelopes).Should(HaveLen(10))
		})

		It("ignores the legacy selector if both old and new selectors are used", func() {
			receiverServer := newSpyBatchedReceiverServer(nil)
			receiver := newSpyReceiver(10)
			server := egress.NewServer(
				receiver,
				testhelper.NewMetricClient(),
				newSpyHealthRegistrar(),
				context.TODO(),
				1,
				time.Nanosecond,
			)

			err := server.BatchedReceiver(&v2.EgressBatchRequest{
				LegacySelector: &v2.Selector{
					SourceId: "source-id",
				},
				Selectors: []*v2.Selector{
					{SourceId: "source-id"},
					{SourceId: "other-source-id"},
				},
			}, receiverServer)
			Expect(err).ToNot(HaveOccurred())

			var req *v2.EgressBatchRequest
			Expect(receiver.requests).Should(Receive(&req))
			Expect(req.Selectors).Should(HaveLen(2))

			Expect(req.LegacySelector).To(BeNil())
		})

		It("upgrades LegacySelector to Selector if no Selectors are present for batching", func() {
			receiverServer := newSpyBatchedReceiverServer(nil)
			receiver := newSpyReceiver(10)
			server := egress.NewServer(
				receiver,
				testhelper.NewMetricClient(),
				newSpyHealthRegistrar(),
				context.TODO(),
				1,
				time.Nanosecond,
			)

			err := server.BatchedReceiver(&v2.EgressBatchRequest{
				LegacySelector: &v2.Selector{
					SourceId: "legacy-source-id",
				},
			}, receiverServer)
			Expect(err).ToNot(HaveOccurred())

			var req *v2.EgressBatchRequest
			Expect(receiver.requests).Should(Receive(&req))
			Expect(req.Selectors).Should(HaveLen(1))

			Expect(req.LegacySelector).To(BeNil())
			Expect(req.Selectors[0].SourceId).To(Equal("legacy-source-id"))
		})

		It("passes the egress batch request through", func() {
			receiverServer := newSpyBatchedReceiverServer(nil)
			receiver := newSpyReceiver(10)
			server := egress.NewServer(
				receiver,
				testhelper.NewMetricClient(),
				newSpyHealthRegistrar(),
				context.TODO(),
				1,
				time.Nanosecond,
			)

			expectedReq := &v2.EgressBatchRequest{
				ShardId: "a-shard-id",
				Selectors: []*v2.Selector{
					{SourceId: "a-source-id"},
				},
				UsePreferredTags: true,
			}
			err := server.BatchedReceiver(expectedReq, receiverServer)
			Expect(err).ToNot(HaveOccurred())

			Eventually(receiver.requests).Should(Receive(Equal(expectedReq)))
		})

		It("closes the receiver when the context is canceled", func() {
			receiverServer := newSpyBatchedReceiverServer(nil)
			receiver := newSpyReceiver(1000000000)

			ctx, cancel := context.WithCancel(context.TODO())
			server := egress.NewServer(
				receiver,
				testhelper.NewMetricClient(),
				newSpyHealthRegistrar(),
				ctx,
				1,
				time.Nanosecond,
			)

			go func() {
				err := server.BatchedReceiver(&v2.EgressBatchRequest{}, receiverServer)
				Expect(err).ToNot(HaveOccurred())
			}()

			cancel()

			var rxCtx context.Context
			Eventually(receiver.ctx).Should(Receive(&rxCtx))
			Eventually(rxCtx.Done).Should(BeClosed())
		})

		It("cancels the context when Receiver exits", func() {
			receiverServer := newSpyBatchedReceiverServer(errors.New("Oh no!"))
			receiver := newSpyReceiver(100000000)
			server := egress.NewServer(
				receiver,
				testhelper.NewMetricClient(),
				newSpyHealthRegistrar(),
				context.TODO(),
				1,
				time.Nanosecond,
			)
			go server.BatchedReceiver(&v2.EgressBatchRequest{}, receiverServer)

			var ctx context.Context
			Eventually(receiver.ctx).Should(Receive(&ctx))
			Eventually(ctx.Done()).Should(BeClosed())
		})

		Describe("Preferred Tags option", func() {
			var (
				receiverServer *spyBatchedReceiverServer
				receiver       *spyReceiver
				server         *egress.Server
			)

			BeforeEach(func() {
				receiverServer = newSpyBatchedReceiverServer(nil)
				receiver = newSpyReceiver(10)
				receiver.envelope = &v2.Envelope{
					Tags: map[string]string{
						"a": "value-a",
					},
					DeprecatedTags: map[string]*v2.Value{
						"b": {
							Data: &v2.Value_Decimal{
								Decimal: 0.8,
							},
						},
						"c": {
							Data: &v2.Value_Integer{
								Integer: 18,
							},
						},
						"d": {
							Data: &v2.Value_Text{
								Text: "value-d",
							},
						},
					},
				}
				server = egress.NewServer(
					receiver,
					testhelper.NewMetricClient(),
					newSpyHealthRegistrar(),
					context.TODO(),
					1,
					time.Nanosecond,
				)
			})

			It("sends deprecated tags", func() {
				err := server.BatchedReceiver(&v2.EgressBatchRequest{
					Selectors:        []*v2.Selector{},
					UsePreferredTags: false,
				}, receiverServer)
				Expect(err).ToNot(HaveOccurred())

				var e *v2.Envelope
				Eventually(receiverServer.envelopes).Should(Receive(&e))
				Expect(e.GetTags()).To(BeEmpty())
				Expect(e.GetDeprecatedTags()).To(HaveLen(4))

				tags := e.GetDeprecatedTags()
				Expect(tags["a"].GetText()).To(Equal("value-a"))
				Expect(tags["b"].GetDecimal()).To(Equal(0.8))
				Expect(tags["c"].GetInteger()).To(Equal(int64(18)))
				Expect(tags["d"].GetText()).To(Equal("value-d"))
			})

			It("sends preferred tags", func() {
				err := server.BatchedReceiver(&v2.EgressBatchRequest{
					Selectors:        []*v2.Selector{},
					UsePreferredTags: true,
				}, receiverServer)
				Expect(err).ToNot(HaveOccurred())

				var e *v2.Envelope
				Eventually(receiverServer.envelopes).Should(Receive(&e))
				Expect(e.GetDeprecatedTags()).To(BeEmpty())
				Expect(e.GetTags()).To(HaveLen(4))

				tags := e.GetTags()
				Expect(tags["a"]).To(Equal("value-a"))
				Expect(tags["b"]).To(Equal("0.8"))
				Expect(tags["c"]).To(Equal("18"))
				Expect(tags["d"]).To(Equal("value-d"))
			})
		})

		Describe("Metrics", func() {
			It("emits 'egress' metric for each envelope", func() {
				metricClient := testhelper.NewMetricClient()
				receiver := newSpyReceiver(10)
				server := egress.NewServer(
					receiver,
					metricClient,
					newSpyHealthRegistrar(),
					context.TODO(),
					10,
					time.Second,
				)

				err := server.BatchedReceiver(
					&v2.EgressBatchRequest{},
					newSpyBatchedReceiverServer(nil),
				)

				Expect(err).ToNot(HaveOccurred())
				Eventually(func() uint64 {
					return metricClient.GetDelta("egress")
				}).Should(BeNumerically("==", 10))
			})

			It("emits 'dropped' metric for each envelope", func() {
				metricClient := testhelper.NewMetricClient()
				receiverServer := newSpyBatchedReceiverServer(nil)
				receiver := newSpyReceiver(1000000)

				server := egress.NewServer(
					receiver,
					metricClient,
					newSpyHealthRegistrar(),
					context.TODO(),
					1,
					time.Nanosecond,
				)
				go server.BatchedReceiver(&v2.EgressBatchRequest{}, receiverServer)

				Eventually(func() uint64 {
					return metricClient.GetDelta("dropped")
				}, 3).Should(BeNumerically(">", 100))
			})
		})

		Describe("health monitoring", func() {
			It("increments and decrements subscription count", func() {
				receiver := newSpyReceiver(1000000000)
				health := newSpyHealthRegistrar()
				server := egress.NewServer(
					receiver,
					testhelper.NewMetricClient(),
					health,
					context.TODO(),
					1,
					time.Nanosecond,
				)

				go server.BatchedReceiver(
					&v2.EgressBatchRequest{},
					newSpyBatchedReceiverServer(nil),
				)

				Eventually(func() float64 {
					return health.Get("subscriptionCount")
				}).Should(Equal(1.0))

				receiver.stop()

				Eventually(func() float64 {
					return health.Get("subscriptionCount")
				}).Should(Equal(0.0))
			})
		})
	})
})

type spyReceiverServer struct {
	err       error
	wait      chan struct{}
	envelopes chan *v2.Envelope

	grpc.ServerStream
}

func newSpyReceiverServer(err error) *spyReceiverServer {
	return &spyReceiverServer{
		envelopes: make(chan *v2.Envelope, 100),
		err:       err,
	}
}

func (*spyReceiverServer) Context() context.Context {
	return context.Background()
}

func (s *spyReceiverServer) Send(e *v2.Envelope) error {
	if s.wait != nil {
		<-s.wait
		return nil
	}

	select {
	case s.envelopes <- e:
	default:
	}

	return s.err
}

func (s *spyReceiverServer) stopWait() {
	close(s.wait)
}

type spyBatchedReceiverServer struct {
	err       error
	envelopes chan *v2.Envelope

	grpc.ServerStream
}

func newSpyBatchedReceiverServer(err error) *spyBatchedReceiverServer {
	return &spyBatchedReceiverServer{
		envelopes: make(chan *v2.Envelope, 1000),
		err:       err,
	}
}

func (*spyBatchedReceiverServer) Context() context.Context {
	return context.Background()
}

func (s *spyBatchedReceiverServer) Send(b *v2.EnvelopeBatch) error {
	for _, e := range b.GetBatch() {
		select {
		case s.envelopes <- e:
		default:
		}
	}

	return s.err
}

type spyReceiver struct {
	envelope       *v2.Envelope
	envelopeRepeat int

	stopCh   chan struct{}
	ctx      chan context.Context
	requests chan *v2.EgressBatchRequest
}

func newSpyReceiver(envelopeCount int) *spyReceiver {
	return &spyReceiver{
		envelope:       &v2.Envelope{},
		envelopeRepeat: envelopeCount,
		stopCh:         make(chan struct{}),
		ctx:            make(chan context.Context, 1),
		requests:       make(chan *v2.EgressBatchRequest, 100),
	}
}

func (s *spyReceiver) Subscribe(ctx context.Context, req *v2.EgressBatchRequest) (func() (*v2.Envelope, error), error) {
	s.ctx <- ctx
	s.requests <- req

	return func() (*v2.Envelope, error) {
		if s.envelopeRepeat > 0 {
			select {
			case <-s.stopCh:
				return nil, io.EOF
			default:
				s.envelopeRepeat--
				return s.envelope, nil
			}
		}

		return nil, errors.New("Oh no!")
	}, nil
}

type stubReceiver struct{}

func (s *stubReceiver) Subscribe(ctx context.Context, req *v2.EgressBatchRequest) (func() (*v2.Envelope, error), error) {
	rx := func() (*v2.Envelope, error) {
		return &v2.Envelope{}, nil
	}
	return rx, nil
}

func (s *spyReceiver) stop() {
	close(s.stopCh)
}

type SpyHealthRegistrar struct {
	mu     sync.Mutex
	values map[string]float64
}

func newSpyHealthRegistrar() *SpyHealthRegistrar {
	return &SpyHealthRegistrar{
		values: make(map[string]float64),
	}
}

func (s *SpyHealthRegistrar) Inc(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[name]++
}

func (s *SpyHealthRegistrar) Dec(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[name]--
}

func (s *SpyHealthRegistrar) Get(name string) float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.values[name]
}
