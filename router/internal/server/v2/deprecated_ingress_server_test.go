package v2_test

import (
	"io"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator/diodes"
	"code.cloudfoundry.org/loggregator/metricemitter"
	"code.cloudfoundry.org/loggregator/router/internal/server/v2"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("DeprecatedIngressServer", func() {
	var (
		v1Buf           *diodes.ManyToOneEnvelope
		v2Buf           *diodes.ManyToOneEnvelopeV2
		mockSender      *mockDopplerIngress_SenderServer
		mockBatchSender *mockBatcherSenderServer
		healthRegistrar *SpyHealthRegistrar

		ingestor *v2.DeprecatedIngressServer
	)

	BeforeEach(func() {
		v1Buf = diodes.NewManyToOneEnvelope(5, nil)
		v2Buf = diodes.NewManyToOneEnvelopeV2(5, nil)
		mockSender = newMockDopplerIngress_SenderServer()
		mockBatchSender = newMockBatcherSenderServer()
		healthRegistrar = newSpyHealthRegistrar()

		ingestor = v2.NewDeprecatedIngressServer(
			v1Buf,
			v2Buf,
			metricemitter.NewCounter("ingress", "doppler"),
			healthRegistrar,
		)
	})

	It("writes batches to the data setter", func() {
		mockBatchSender.RecvOutput.Ret0 <- &loggregator_v2.EnvelopeBatch{
			Batch: []*loggregator_v2.Envelope{
				{
					Message: &loggregator_v2.Envelope_Log{
						Log: &loggregator_v2.Log{
							Payload: []byte("hello-1"),
						},
					},
				},
				{
					Message: &loggregator_v2.Envelope_Log{
						Log: &loggregator_v2.Log{
							Payload: []byte("hello-2"),
						},
					},
				},
			},
		}

		mockBatchSender.RecvOutput.Ret1 <- nil
		mockBatchSender.RecvOutput.Ret0 <- nil
		mockBatchSender.RecvOutput.Ret1 <- io.EOF

		ingestor.BatchSender(mockBatchSender)

		_, ok := v1Buf.TryNext()
		Expect(ok).To(BeTrue())
		_, ok = v2Buf.TryNext()
		Expect(ok).To(BeTrue())

		_, ok = v1Buf.TryNext()
		Expect(ok).To(BeTrue())
		_, ok = v2Buf.TryNext()
		Expect(ok).To(BeTrue())
	})

	It("writes a single envelope to the data setter", func() {
		mockSender.RecvOutput.Ret0 <- &loggregator_v2.Envelope{
			Message: &loggregator_v2.Envelope_Log{
				Log: &loggregator_v2.Log{
					Payload: []byte("hello"),
				},
			},
		}
		mockSender.RecvOutput.Ret1 <- nil
		mockSender.RecvOutput.Ret0 <- nil
		mockSender.RecvOutput.Ret1 <- io.EOF

		ingestor.Sender(mockSender)

		_, ok := v1Buf.TryNext()
		Expect(ok).To(BeTrue())
		_, ok = v2Buf.TryNext()
		Expect(ok).To(BeTrue())
	})

	It("throws invalid envelopes on the ground", func() {
		mockSender.RecvOutput.Ret0 <- &loggregator_v2.Envelope{}
		mockSender.RecvOutput.Ret1 <- nil
		mockSender.RecvOutput.Ret0 <- nil
		mockSender.RecvOutput.Ret1 <- io.EOF

		ingestor.Sender(mockSender)
		_, ok := v1Buf.TryNext()
		Expect(ok).ToNot(BeTrue())
	})

	Describe("health monitoring", func() {
		Describe("Sender()", func() {
			It("increments and decrements the number of ingress streams", func() {
				go ingestor.Sender(mockSender)

				Eventually(func() float64 {
					return healthRegistrar.Get("ingressStreamCount")
				}).Should(Equal(1.0))

				mockSender.RecvOutput.Ret0 <- nil
				mockSender.RecvOutput.Ret1 <- io.EOF

				Eventually(func() float64 {
					return healthRegistrar.Get("ingressStreamCount")
				}).Should(Equal(0.0))
			})
		})

		Describe("BatchSender()", func() {
			It("increments and decrements the number of ingress streams", func() {
				go ingestor.BatchSender(mockBatchSender)

				Eventually(func() float64 {
					return healthRegistrar.Get("ingressStreamCount")
				}).Should(Equal(1.0))

				mockBatchSender.RecvOutput.Ret0 <- nil
				mockBatchSender.RecvOutput.Ret1 <- io.EOF

				Eventually(func() float64 {
					return healthRegistrar.Get("ingressStreamCount")
				}).Should(Equal(0.0))
			})
		})
	})
})
