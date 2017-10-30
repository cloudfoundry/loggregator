package v2_test

import (
	"context"
	"errors"
	"io"

	"code.cloudfoundry.org/loggregator/metricemitter/testhelper"

	v2 "code.cloudfoundry.org/loggregator/plumbing/v2"

	ingress "code.cloudfoundry.org/loggregator/agent/internal/ingress/v2"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Receiver", func() {
	var (
		rx           *ingress.Receiver
		spySetter    *SpySetter
		metricClient *testhelper.SpyMetricClient
	)

	BeforeEach(func() {
		spySetter = NewSpySetter()
		metricClient = testhelper.NewMetricClient()
		rx = ingress.NewReceiver(spySetter, metricClient)
	})

	Describe("Sender()", func() {
		var (
			spySender *SpySender
		)

		BeforeEach(func() {
			spySender = NewSpySender()
		})

		It("calls set on the data setter with the data", func() {
			e := &v2.Envelope{
				SourceId: "some-id",
			}

			spySender.recvResponses <- SenderRecvResponse{
				envelope: e,
			}
			spySender.recvResponses <- SenderRecvResponse{
				envelope: e,
			}
			spySender.recvResponses <- SenderRecvResponse{
				err: io.EOF,
			}

			rx.Sender(spySender)

			Expect(spySetter.envelopes).To(Receive(Equal(e)))
			Expect(spySetter.envelopes).To(Receive(Equal(e)))
		})

		It("returns an error when receive fails", func() {
			spySender.recvResponses <- SenderRecvResponse{
				err: errors.New("error occurred"),
			}

			err := rx.Sender(spySender)

			Expect(err).To(HaveOccurred())
		})

		It("increments the ingress metric", func() {
			e := &v2.Envelope{
				SourceId: "some-id",
			}

			spySender.recvResponses <- SenderRecvResponse{
				envelope: e,
			}
			spySender.recvResponses <- SenderRecvResponse{
				envelope: e,
			}
			spySender.recvResponses <- SenderRecvResponse{
				err: io.EOF,
			}

			rx.Sender(spySender)

			Expect(metricClient.GetDelta("ingress")).To(Equal(uint64(2)))
		})
	})

	Describe("BatchSender()", func() {
		var (
			spyBatchSender *SpyBatchSender
		)

		BeforeEach(func() {
			spyBatchSender = NewSpyBatchSender()
		})

		It("calls set on the datasetting with all the envelopes", func() {
			e := &v2.Envelope{
				SourceId: "some-id",
			}

			spyBatchSender.recvResponses <- BatchSenderRecvResponse{
				envelopes: []*v2.Envelope{e, e, e, e, e},
			}
			spyBatchSender.recvResponses <- BatchSenderRecvResponse{
				err: io.EOF,
			}

			rx.BatchSender(spyBatchSender)

			Expect(spySetter.envelopes).Should(HaveLen(5))
		})

		It("returns an error when receive fails", func() {
			spyBatchSender.recvResponses <- BatchSenderRecvResponse{
				err: errors.New("error occurred"),
			}

			err := rx.BatchSender(spyBatchSender)

			Expect(err).To(HaveOccurred())
		})

		It("increments the ingress metric", func() {
			e := &v2.Envelope{
				SourceId: "some-id",
			}

			spyBatchSender.recvResponses <- BatchSenderRecvResponse{
				envelopes: []*v2.Envelope{e, e, e, e, e},
			}
			spyBatchSender.recvResponses <- BatchSenderRecvResponse{
				err: io.EOF,
			}

			rx.BatchSender(spyBatchSender)

			Expect(spySetter.envelopes).Should(HaveLen(5))

			Expect(metricClient.GetDelta("ingress")).To(Equal(uint64(5)))
		})
	})

	Describe("Send()", func() {
		It("calls set on the setter with the given envelopes", func() {
			e1 := &v2.Envelope{
				SourceId: "some-id-1",
			}
			e2 := &v2.Envelope{
				SourceId: "some-id-2",
			}

			rx.Send(context.Background(), &v2.EnvelopeBatch{
				Batch: []*v2.Envelope{e1, e2},
			})

			Expect(spySetter.envelopes).To(Receive(Equal(e1)))
			Expect(spySetter.envelopes).To(Receive(Equal(e2)))
		})

		It("increments the ingress metric", func() {
			e := &v2.Envelope{
				SourceId: "some-id",
			}

			rx.Send(context.Background(), &v2.EnvelopeBatch{
				Batch: []*v2.Envelope{e},
			})

			Expect(metricClient.GetDelta("ingress")).To(Equal(uint64(1)))
		})
	})
})

type SenderRecvResponse struct {
	envelope *v2.Envelope
	err      error
}

type BatchSenderRecvResponse struct {
	envelopes []*v2.Envelope
	err       error
}

type SpySender struct {
	v2.Ingress_SenderServer
	recvResponses chan SenderRecvResponse
}

func NewSpySender() *SpySender {
	return &SpySender{
		recvResponses: make(chan SenderRecvResponse, 100),
	}
}

func (s *SpySender) Recv() (*v2.Envelope, error) {
	resp := <-s.recvResponses

	return resp.envelope, resp.err
}

type SpyBatchSender struct {
	v2.Ingress_BatchSenderServer
	recvResponses chan BatchSenderRecvResponse
}

func NewSpyBatchSender() *SpyBatchSender {
	return &SpyBatchSender{
		recvResponses: make(chan BatchSenderRecvResponse, 100),
	}
}

func (s *SpyBatchSender) Recv() (*v2.EnvelopeBatch, error) {
	resp := <-s.recvResponses

	return &v2.EnvelopeBatch{Batch: resp.envelopes}, resp.err
}

type SpySetter struct {
	envelopes chan *v2.Envelope
}

func NewSpySetter() *SpySetter {
	return &SpySetter{
		envelopes: make(chan *v2.Envelope, 100),
	}
}

func (s *SpySetter) Set(e *v2.Envelope) {
	s.envelopes <- e
}
