package web

import (
	"context"
	"net/http"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
)

type Receiver func() (*loggregator_v2.EnvelopeBatch, error)

type LogsProvider interface {
	Stream(ctx context.Context, req *loggregator_v2.EgressBatchRequest) Receiver
}

type Handler struct {
	mux *http.ServeMux
}

func NewHandler(lp LogsProvider) *Handler {
	mux := http.NewServeMux()

	mux.Handle("/v2/read", ReadHandler(lp))

	return &Handler{mux: mux}
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}
