package v2_test

import (
	"fmt"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	egress "code.cloudfoundry.org/loggregator/agent/internal/egress/v2"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Counteraggregator", func() {
	It("forwards non-counter enevelopes", func() {
		mockWriter := newMockWriter()
		close(mockWriter.WriteOutput.Ret0)

		logEnvelope := &loggregator_v2.Envelope{
			Message: &loggregator_v2.Envelope_Log{
				Log: &loggregator_v2.Log{
					Payload: []byte("some-message"),
					Type:    loggregator_v2.Log_OUT,
				},
			},
		}

		aggregator := egress.NewCounterAggregator(mockWriter)
		aggregator.Write([]*loggregator_v2.Envelope{logEnvelope})

		Expect(mockWriter.WriteInput.Msg).To(Receive(Equal([]*loggregator_v2.Envelope{logEnvelope})))
	})

	It("calculates totals for same counter envelopes", func() {
		mockWriter := newMockWriter()
		close(mockWriter.WriteOutput.Ret0)

		aggregator := egress.NewCounterAggregator(mockWriter)
		aggregator.Write(buildCounterEnvelope(10, "name-1", "origin-1"))
		aggregator.Write(buildCounterEnvelope(15, "name-1", "origin-1"))

		var receivedEnvelope []*loggregator_v2.Envelope
		Expect(mockWriter.WriteInput.Msg).To(Receive(&receivedEnvelope))
		Expect(receivedEnvelope).To(HaveLen(1))
		Expect(receivedEnvelope[0].GetCounter().GetTotal()).To(Equal(uint64(10)))

		Expect(mockWriter.WriteInput.Msg).To(Receive(&receivedEnvelope))
		Expect(receivedEnvelope).To(HaveLen(1))
		Expect(receivedEnvelope[0].GetCounter().GetTotal()).To(Equal(uint64(25)))
	})

	It("calculates totals separately for counter envelopes with unique names", func() {
		mockWriter := newMockWriter()
		close(mockWriter.WriteOutput.Ret0)

		aggregator := egress.NewCounterAggregator(mockWriter)
		aggregator.Write(buildCounterEnvelope(10, "name-1", "origin-1"))
		aggregator.Write(buildCounterEnvelope(15, "name-2", "origin-1"))
		aggregator.Write(buildCounterEnvelope(20, "name-3", "origin-1"))

		var receivedEnvelope []*loggregator_v2.Envelope
		Expect(mockWriter.WriteInput.Msg).To(Receive(&receivedEnvelope))
		Expect(receivedEnvelope).To(HaveLen(1))
		Expect(receivedEnvelope[0].GetCounter().GetTotal()).To(Equal(uint64(10)))

		Expect(mockWriter.WriteInput.Msg).To(Receive(&receivedEnvelope))
		Expect(receivedEnvelope).To(HaveLen(1))
		Expect(receivedEnvelope[0].GetCounter().GetTotal()).To(Equal(uint64(15)))

		Expect(mockWriter.WriteInput.Msg).To(Receive(&receivedEnvelope))
		Expect(receivedEnvelope).To(HaveLen(1))
		Expect(receivedEnvelope[0].GetCounter().GetTotal()).To(Equal(uint64(20)))
	})

	It("calculates totals separately for counter envelopes with same name but unique tags", func() {
		mockWriter := newMockWriter()
		close(mockWriter.WriteOutput.Ret0)

		aggregator := egress.NewCounterAggregator(mockWriter)
		aggregator.Write(buildCounterEnvelope(10, "name-1", "origin-1"))
		aggregator.Write(buildCounterEnvelope(15, "name-1", "origin-1"))
		aggregator.Write(buildCounterEnvelope(20, "name-1", "origin-2"))

		var receivedEnvelope []*loggregator_v2.Envelope
		Expect(mockWriter.WriteInput.Msg).To(Receive(&receivedEnvelope))
		Expect(receivedEnvelope).To(HaveLen(1))
		Expect(receivedEnvelope[0].GetCounter().GetTotal()).To(Equal(uint64(10)))

		Expect(mockWriter.WriteInput.Msg).To(Receive(&receivedEnvelope))
		Expect(receivedEnvelope).To(HaveLen(1))
		Expect(receivedEnvelope[0].GetCounter().GetTotal()).To(Equal(uint64(25)))

		Expect(mockWriter.WriteInput.Msg).To(Receive(&receivedEnvelope))
		Expect(receivedEnvelope).To(HaveLen(1))
		Expect(receivedEnvelope[0].GetCounter().GetTotal()).To(Equal(uint64(20)))
	})

	It("calculations are unaffected for counter envelopes with total set", func() {
		mockWriter := newMockWriter()
		close(mockWriter.WriteOutput.Ret0)

		aggregator := egress.NewCounterAggregator(mockWriter)
		aggregator.Write(buildCounterEnvelope(10, "name-1", "origin-1"))
		aggregator.Write(buildCounterEnvelopeWithTotal(5000, "name-1", "origin-1"))

		var receivedEnvelope []*loggregator_v2.Envelope
		Expect(mockWriter.WriteInput.Msg).To(Receive(&receivedEnvelope))
		Expect(receivedEnvelope).To(HaveLen(1))
		Expect(receivedEnvelope[0].GetCounter().GetTotal()).To(Equal(uint64(10)))

		Expect(mockWriter.WriteInput.Msg).To(Receive(&receivedEnvelope))
		Expect(receivedEnvelope).To(HaveLen(1))
		Expect(receivedEnvelope[0].GetCounter().GetTotal()).To(Equal(uint64(10)))
	})

	It("prunes the cache of totals when there are too many unique counters", func() {
		mockWriter := newMockWriter()
		close(mockWriter.WriteOutput.Ret0)

		aggregator := egress.NewCounterAggregator(mockWriter)

		aggregator.Write(buildCounterEnvelope(500, "unique-name", "origin-1"))

		var receivedEnvelope []*loggregator_v2.Envelope
		Expect(mockWriter.WriteInput.Msg).To(Receive(&receivedEnvelope))
		Expect(receivedEnvelope).To(HaveLen(1))
		Expect(receivedEnvelope[0].GetCounter().GetTotal()).To(Equal(uint64(500)))

		for i := 0; i < 10000; i++ {
			aggregator.Write(buildCounterEnvelope(10, fmt.Sprint("name-", i), "origin-1"))
			<-mockWriter.WriteInput.Msg
			<-mockWriter.WriteCalled
		}

		aggregator.Write(buildCounterEnvelope(10, "unique-name", "origin-1"))

		Expect(mockWriter.WriteInput.Msg).To(Receive(&receivedEnvelope))
		Expect(receivedEnvelope).To(HaveLen(1))
		Expect(receivedEnvelope[0].GetCounter().GetTotal()).To(Equal(uint64(10)))
	})

	It("keeps the delta as part of the message", func() {
		mockWriter := newMockWriter()
		close(mockWriter.WriteOutput.Ret0)

		aggregator := egress.NewCounterAggregator(mockWriter)
		aggregator.Write(buildCounterEnvelope(10, "name-1", "origin-1"))

		var receivedEnvelopes []*loggregator_v2.Envelope
		Expect(mockWriter.WriteInput.Msg).To(Receive(&receivedEnvelopes))
		Expect(receivedEnvelopes).To(HaveLen(1))
		Expect(receivedEnvelopes[0].GetCounter().GetDelta()).To(Equal(uint64(10)))
	})
})

func buildCounterEnvelope(delta uint64, name, origin string) []*loggregator_v2.Envelope {
	return []*loggregator_v2.Envelope{{
		Message: &loggregator_v2.Envelope_Counter{
			Counter: &loggregator_v2.Counter{
				Name:  name,
				Delta: delta,
			},
		},
		Tags: map[string]string{
			"origin": origin,
		},
	}}
}

func buildCounterEnvelopeWithTotal(total uint64, name, origin string) []*loggregator_v2.Envelope {
	return []*loggregator_v2.Envelope{{
		Message: &loggregator_v2.Envelope_Counter{
			Counter: &loggregator_v2.Counter{
				Name:  name,
				Total: total,
			},
		},
		Tags: map[string]string{
			"origin": origin,
		},
	}}
}
