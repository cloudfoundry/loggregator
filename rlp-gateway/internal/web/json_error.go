package web

import (
	"bytes"
	"encoding/json"
)

var (
	errMethodNotAllowed           = newJSONError("method_not_allowed", "method not allowed")
	errEmptyQuery                 = newJSONError("empty_query", "query cannot be empty")
	errMissingType                = newJSONError("missing_envelope_type", "query must provide at least one envelope type")
	errCounterNamePresentButEmpty = newJSONError("missing_counter_name", "counter.name is invalid without value")
	errGaugeNamePresentButEmpty   = newJSONError("missing_gauge_name", "gauge.name is invalid without value")
	errStreamingUnsupported       = newJSONError("streaming_unsupported", "request does not support streaming")
	errNotFound                   = newJSONError("not_found", "resource not found")
)

type jsonError struct {
	Name    string `json:"error"`
	Message string `json:"message"`
}

func newJSONError(name, msg string) jsonError {
	return jsonError{
		Name:    name,
		Message: msg,
	}
}

func (e jsonError) Error() string {
	buf := bytes.NewBuffer(nil)

	// We control this struct and it should never fail to encode.
	_ = json.NewEncoder(buf).Encode(&e)

	return buf.String()
}
