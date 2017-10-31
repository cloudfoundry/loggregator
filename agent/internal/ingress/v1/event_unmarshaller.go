package v1

import (
	"errors"
	"log"

	"github.com/cloudfoundry/sonde-go/events"
	"github.com/gogo/protobuf/proto"
)

type EnvelopeWriter interface {
	Write(event *events.Envelope)
}

var (
	invalidEnvelope = errors.New("Invalid Envelope")
)

// An EventUnmarshaller is an self-instrumenting tool for converting Protocol
// Buffer-encoded dropsonde messages to Envelope instances.
type EventUnmarshaller struct {
	outputWriter EnvelopeWriter
}

func NewUnMarshaller(outputWriter EnvelopeWriter) *EventUnmarshaller {
	return &EventUnmarshaller{
		outputWriter: outputWriter,
	}
}

func (u *EventUnmarshaller) Write(message []byte) {
	envelope, err := u.UnmarshallMessage(message)
	if err != nil {
		log.Printf("Error unmarshalling: %s", err)
		return
	}
	u.outputWriter.Write(envelope)
}

func (u *EventUnmarshaller) UnmarshallMessage(message []byte) (*events.Envelope, error) {
	envelope := &events.Envelope{}
	err := proto.Unmarshal(message, envelope)
	if err != nil {
		log.Printf("eventUnmarshaller: unmarshal error %v", err)
		return nil, err
	}

	if !valid(envelope) {
		log.Printf("eventUnmarshaller: validation failed for message %v", envelope.GetEventType())
		return nil, invalidEnvelope
	}

	return envelope, nil
}

func valid(env *events.Envelope) bool {
	switch env.GetEventType() {
	case events.Envelope_HttpStartStop:
		return env.GetHttpStartStop() != nil
	case events.Envelope_LogMessage:
		return env.GetLogMessage() != nil
	case events.Envelope_ValueMetric:
		return env.GetValueMetric() != nil
	case events.Envelope_CounterEvent:
		return env.GetCounterEvent() != nil
	case events.Envelope_Error:
		return env.GetError() != nil
	case events.Envelope_ContainerMetric:
		return env.GetContainerMetric() != nil
	default:
		return false
	}
}
