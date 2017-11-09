package v1

import (
	"errors"
	"sync"
	"time"

	"github.com/cloudfoundry/sonde-go/events"
	"github.com/gogo/protobuf/proto"
)

type EventWriter struct {
	origin string
	writer EnvelopeWriter
	lock   sync.RWMutex
}

func New(origin string) *EventWriter {
	return &EventWriter{
		origin: origin,
	}
}

func (e *EventWriter) Emit(event events.Event) error {
	envelope, err := wrap(event, e.origin)
	if err != nil {
		return err
	}

	return e.EmitEnvelope(envelope)
}

func (e *EventWriter) EmitEnvelope(envelope *events.Envelope) error {
	e.lock.RLock()
	defer e.lock.RUnlock()
	if e.writer == nil {
		return errors.New("EventWriter: No envelope writer set (see SetWriter)")
	}
	e.writer.Write(envelope)
	return nil
}

func (e *EventWriter) Origin() string {
	return e.origin
}

func (e *EventWriter) SetWriter(writer EnvelopeWriter) {
	e.lock.Lock()
	defer e.lock.Unlock()
	e.writer = writer
}

var ErrorMissingOrigin = errors.New("Event not emitted due to missing origin information")
var ErrorUnknownEventType = errors.New("Cannot create envelope for unknown event type")

func wrap(event events.Event, origin string) (*events.Envelope, error) {
	if origin == "" {
		return nil, ErrorMissingOrigin
	}

	envelope := &events.Envelope{Origin: proto.String(origin), Timestamp: proto.Int64(time.Now().UnixNano())}

	switch event := event.(type) {
	case *events.HttpStartStop:
		envelope.EventType = events.Envelope_HttpStartStop.Enum()
		envelope.HttpStartStop = event
	case *events.ValueMetric:
		envelope.EventType = events.Envelope_ValueMetric.Enum()
		envelope.ValueMetric = event
	case *events.CounterEvent:
		envelope.EventType = events.Envelope_CounterEvent.Enum()
		envelope.CounterEvent = event
	case *events.LogMessage:
		envelope.EventType = events.Envelope_LogMessage.Enum()
		envelope.LogMessage = event
	case *events.ContainerMetric:
		envelope.EventType = events.Envelope_ContainerMetric.Enum()
		envelope.ContainerMetric = event
	default:
		return nil, ErrorUnknownEventType
	}

	return envelope, nil
}
