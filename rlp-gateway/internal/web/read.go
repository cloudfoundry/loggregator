package web

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"github.com/gogo/protobuf/jsonpb"
)

var marshaler jsonpb.Marshaler

func ReadHandler(lp LogsProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		query := r.URL.Query()

		s, err := BuildSelector(query)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		recv := lp.Stream(
			ctx,
			&loggregator_v2.EgressBatchRequest{
				ShardId:           query.Get("shard_id"),
				DeterministicName: query.Get("deterministic_name"),
				Selectors:         s,
			},
		)

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		for {
			if isDone(ctx) {
				w.WriteHeader(http.StatusOK)
				return
			}

			batch, err := recv()
			if err != nil {
				log.Printf("error getting logs from provider: %s", err)
				w.WriteHeader(http.StatusGone)
				return
			}

			data, err := marshaler.MarshalToString(batch)
			if err != nil {
				log.Printf("error marshaling envelope batch to string: %s", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func isDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}
