package trafficcontroller_test

import (
	"fmt"
	"net"
	"sync"
	"time"

	"code.cloudfoundry.org/go-log-cache/rpc/logcache_v1"
	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

type stubGrpcLogCache struct {
	mu       sync.Mutex
	reqs     []*logcache_v1.ReadRequest
	promReqs []*logcache_v1.PromQL_InstantQueryRequest
	lis      net.Listener
	block    bool
}

func newStubGrpcLogCache(port int) *stubGrpcLogCache {
	s := &stubGrpcLogCache{}
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		panic(err)
	}
	s.lis = lis
	srv := grpc.NewServer()
	logcache_v1.RegisterEgressServer(srv, s)
	logcache_v1.RegisterPromQLQuerierServer(srv, s)

	go func() {
		err = srv.Serve(lis)
		if err != nil {
			panic(err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	return s
}

func (s *stubGrpcLogCache) addr() string {
	return s.lis.Addr().String()
}

func (s *stubGrpcLogCache) Read(c context.Context, r *logcache_v1.ReadRequest) (*logcache_v1.ReadResponse, error) {
	if s.block {
		var block chan struct{}
		<-block
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.reqs = append(s.reqs, r)

	return &logcache_v1.ReadResponse{
		Envelopes: &loggregator_v2.EnvelopeBatch{
			Batch: []*loggregator_v2.Envelope{
				{
					Timestamp: 0,
					SourceId:  r.GetSourceId(),
					Message: &loggregator_v2.Envelope_Log{
						Log: &loggregator_v2.Log{
							Payload: []byte("0"),
						},
					},
				},
				{
					Timestamp: 1,
					SourceId:  r.GetSourceId(),
					Message: &loggregator_v2.Envelope_Log{
						Log: &loggregator_v2.Log{
							Payload: []byte("1"),
						},
					},
				},
			},
		},
	}, nil
}

func (s *stubGrpcLogCache) InstantQuery(c context.Context, r *logcache_v1.PromQL_InstantQueryRequest) (*logcache_v1.PromQL_QueryResult, error) {
	panic("InstantQuery is not implemented")
}

func (s *stubGrpcLogCache) Meta(context.Context, *logcache_v1.MetaRequest) (*logcache_v1.MetaResponse, error) {
	panic("Meta is not implemented")
}

func (s *stubGrpcLogCache) requests() []*logcache_v1.ReadRequest {
	s.mu.Lock()
	defer s.mu.Unlock()

	r := make([]*logcache_v1.ReadRequest, len(s.reqs))
	copy(r, s.reqs)
	return r
}
