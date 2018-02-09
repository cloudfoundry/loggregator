package ingress

import (
	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator/plumbing/conversion"

	"github.com/cloudfoundry/sonde-go/events"
)

func NewConverter() EnvelopeConverter {
	return envelopeConverter{}
}

type envelopeConverter struct{}

func (e envelopeConverter) Convert(payload []byte, usePreferredTags bool) (*loggregator_v2.Envelope, error) {
	v1e := &events.Envelope{}
	err := v1e.Unmarshal(payload)
	if err != nil {
		return nil, err
	}

	return conversion.ToV2(v1e, usePreferredTags), nil
}
