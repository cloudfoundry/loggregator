package app

import (
	"log"
	"net"
	"net/http"

	"code.cloudfoundry.org/loggregator/plumbing"
	"code.cloudfoundry.org/loggregator/rlp-gateway/internal/ingress"
	"code.cloudfoundry.org/loggregator/rlp-gateway/internal/web"
)

type Gateway struct {
	cfg      Config
	listener net.Listener
	server   *http.Server
	log      *log.Logger
}

func NewGateway(cfg Config) *Gateway {
	return &Gateway{
		cfg: cfg,
	}
}

func (g *Gateway) Start(blocking bool) {
	l, err := net.Listen("tcp", g.cfg.GatewayAddr)
	if err != nil {
		g.log.Fatalf("failed to start listener: %s", err)
	}

	creds, err := plumbing.NewClientCredentials(
		g.cfg.LogsProviderCertPath,
		g.cfg.LogsProviderKeyPath,
		g.cfg.LogsProviderCAPath,
		g.cfg.LogsProviderCommonName,
	)
	if err != nil {
		log.Fatalf("failed to load client TLS config: %s", err)
	}

	g.listener = l
	g.server = &http.Server{
		Addr:    g.cfg.GatewayAddr,
		Handler: web.NewHandler(ingress.NewLogClient(creds, g.cfg.LogsProviderAddr)),
		// TODO: Logging Middleware
		// TODO: Recovery Middleware
	}

	if blocking {
		g.server.Serve(g.listener)
		return
	}

	go g.server.Serve(g.listener)
}

func (g *Gateway) Stop() {
	_ = g.server.Close()
}

func (g *Gateway) Addr() string {
	if g.listener == nil {
		return ""
	}

	return g.listener.Addr().String()
}
