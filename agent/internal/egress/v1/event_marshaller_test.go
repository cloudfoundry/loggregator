package v1_test

import (
	egress "code.cloudfoundry.org/loggregator/agent/internal/egress/v1"
	"code.cloudfoundry.org/loggregator/metricemitter/testhelper"

	"github.com/cloudfoundry/sonde-go/events"
	"github.com/gogo/protobuf/proto"

	. "github.com/apoydence/eachers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("EventMarshaller", func() {
	var (
		marshaller      *egress.EventMarshaller
		mockChainWriter *mockBatchChainByteWriter
		metricClient    *testhelper.SpyMetricClient
	)

	BeforeEach(func() {
		mockChainWriter = newMockBatchChainByteWriter()
		metricClient = testhelper.NewMetricClient()
	})

	JustBeforeEach(func() {
		marshaller = egress.NewMarshaller(metricClient)
		marshaller.SetWriter(mockChainWriter)
	})

	Describe("Write", func() {
		var envelope *events.Envelope

		Context("with a nil writer", func() {
			BeforeEach(func() {
				envelope = &events.Envelope{
					Origin:    proto.String("The Negative Zone"),
					EventType: events.Envelope_LogMessage.Enum(),
				}
			})

			JustBeforeEach(func() {
				marshaller.SetWriter(nil)
			})

			It("does not panic", func() {
				Expect(func() {
					marshaller.Write(envelope)
				}).ToNot(Panic())
			})
		})

		Context("with an invalid envelope", func() {
			BeforeEach(func() {
				envelope = &events.Envelope{}
				close(mockChainWriter.WriteOutput.Err)
			})

			It("doesn't write the bytes", func() {
				marshaller.Write(envelope)
				Consistently(mockChainWriter.WriteCalled).ShouldNot(Receive())
			})
		})

		Context("with writer", func() {
			BeforeEach(func() {
				close(mockChainWriter.WriteOutput.Err)
				envelope = &events.Envelope{
					Origin:    proto.String("The Negative Zone"),
					EventType: events.Envelope_LogMessage.Enum(),
				}
			})

			It("writes messages to the writer", func() {
				marshaller.Write(envelope)
				expected, err := proto.Marshal(envelope)
				Expect(err).ToNot(HaveOccurred())
				Expect(mockChainWriter.WriteInput.Message).To(Receive(Equal(expected)))
				Expect(metricClient.GetDelta("egress")).To(Equal(uint64(1)))
			})
		})
	})

	Describe("SetWriter", func() {
		It("writes to the new writer", func() {
			newWriter := newMockBatchChainByteWriter()
			close(newWriter.WriteOutput.Err)
			marshaller.SetWriter(newWriter)

			envelope := &events.Envelope{
				Origin:    proto.String("The Negative Zone"),
				EventType: events.Envelope_LogMessage.Enum(),
			}
			marshaller.Write(envelope)

			expected, err := proto.Marshal(envelope)
			Expect(err).ToNot(HaveOccurred())
			Consistently(mockChainWriter.WriteInput).ShouldNot(BeCalled())
			Eventually(newWriter.WriteInput).Should(BeCalled(With(expected)))
		})
	})
})
