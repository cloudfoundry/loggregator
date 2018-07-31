package web

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"github.com/golang/protobuf/jsonpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var marshaler jsonpb.Marshaler

// ReadHandler returns a http.Handler that will serve logs over server sent
// events. Logs are streamed from the logs provider and written to the client
// connection. The format of the envelopes is as follows:
//
//     data: <JSON ENVELOPE BATCH>
//
//     data: <JSON ENVELOPE BATCH>
func ReadHandler(lp LogsProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, errMethodNotAllowed.Error(), http.StatusMethodNotAllowed)
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
				UsePreferredTags:  true,
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

		flusher.Flush()

		// TODO:
		//   - ping events
		//   - error events
		for {
			if isDone(ctx) {
				return
			}

			batch, err := recv()
			if err != nil {
				status, ok := status.FromError(err)
				if ok && status.Code() != codes.Canceled {
					log.Printf("error getting logs from provider: %s", err)
				}

				return
			}

			data, err := marshaler.MarshalToString(batch)
			if err != nil {
				log.Printf("error marshaling envelope batch to string: %s", err)
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
