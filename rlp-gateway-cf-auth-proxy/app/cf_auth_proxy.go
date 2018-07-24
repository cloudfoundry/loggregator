package app

import (
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// CFAuthProxy defines a proxy to authenticate all incoming requests with CF
type CFAuthProxy struct {
	blockOnStart bool
	ln           net.Listener

	gatewayURL *url.URL
	addr       string

	authMiddleware func(http.Handler) http.Handler
}

// NewCFAuthProxy creates and returns a new CFAuthProxy
func NewCFAuthProxy(gatewayAddr, addr string, opts ...CFAuthProxyOption) *CFAuthProxy {
	gatewayURL, err := url.Parse(gatewayAddr)
	if err != nil {
		log.Fatalf("failed to parse gateway address: %s", err)
	}

	p := &CFAuthProxy{
		gatewayURL: gatewayURL,
		addr:       addr,
		authMiddleware: func(h http.Handler) http.Handler {
			return h
		},
	}

	for _, o := range opts {
		o(p)
	}

	return p
}

// CFAuthProxyOption configures a CFAuthProxy
type CFAuthProxyOption func(*CFAuthProxy)

// WithCFAuthProxyBlock returns a CFAuthProxyOption that determines if Start
// launches a go-routine or not. It defaults to launching a go-routine. If
// this is set, start will block on serving the HTTP endpoint.
func WithCFAuthProxyBlock() CFAuthProxyOption {
	return func(p *CFAuthProxy) {
		p.blockOnStart = true
	}
}

// WithAuthMiddleware returns a CFAuthProxyOption that sets the CFAuthProxy's
// authentication and authorization middleware.
func WithAuthMiddleware(authMiddleware func(http.Handler) http.Handler) CFAuthProxyOption {
	return func(p *CFAuthProxy) {
		p.authMiddleware = authMiddleware
	}
}

// Start starts the HTTP listener and serves the HTTP server. If the
// CFAuthProxy was initialized with the WithCFAuthProxyBlock option this
// method will block.
func (p *CFAuthProxy) Start() {
	ln, err := net.Listen("tcp", p.addr)
	if err != nil {
		log.Fatalf("failed to start listener: %s", err)
	}

	p.ln = ln

	server := http.Server{
		Handler: p.authMiddleware(p.reverseProxy()),
	}

	if p.blockOnStart {
		log.Fatal(server.Serve(ln))
	}

	go func() {
		log.Fatal(server.Serve(ln))
	}()
}

// Addr returns the listener address. This must be called after calling Start.
func (p *CFAuthProxy) Addr() string {
	return p.ln.Addr().String()
}

func (p *CFAuthProxy) reverseProxy() *httputil.ReverseProxy {
	return httputil.NewSingleHostReverseProxy(p.gatewayURL)
}
