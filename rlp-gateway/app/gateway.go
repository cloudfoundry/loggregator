package app

import (
	"log"
	"net"
	"net/http"
	"os"

	"code.cloudfoundry.org/loggregator/plumbing"
	"code.cloudfoundry.org/loggregator/rlp-gateway/internal/ingress"
	"code.cloudfoundry.org/loggregator/rlp-gateway/internal/web"
	"github.com/gorilla/handlers"
)

// Gateway provides a high level for running the RLP gateway
type Gateway struct {
	cfg      Config
	listener net.Listener
	server   *http.Server
}

// NewGateway creates a new Gateway
func NewGateway(cfg Config) *Gateway {
	return &Gateway{
		cfg: cfg,
	}
}

// Start will start the process that connects to the logs provider
// and listens on http
func (g *Gateway) Start(blocking bool) {
	creds, err := plumbing.NewClientCredentials(
		g.cfg.LogsProviderClientCertPath,
		g.cfg.LogsProviderClientKeyPath,
		g.cfg.LogsProviderCAPath,
		g.cfg.LogsProviderCommonName,
	)
	if err != nil {
		log.Fatalf("failed to load client TLS config: %s", err)
	}

	lc := ingress.NewLogClient(creds, g.cfg.LogsProviderAddr)
	stack := handlers.RecoveryHandler(handlers.PrintRecoveryStack(true))(
		handlers.LoggingHandler(os.Stdout, web.NewHandler(lc)),
	)

	l, err := net.Listen("tcp", g.cfg.GatewayAddr)
	if err != nil {
		log.Fatalf("failed to start listener: %s", err)
	}
	log.Printf("http bound to: %s", l.Addr().String())

	g.listener = l
	g.server = &http.Server{
		Addr:    g.cfg.GatewayAddr,
		Handler: stack,
	}

	if blocking {
		g.server.Serve(g.listener)
		return
	}

	go g.server.Serve(g.listener)
}

// Stop closes the server connection
func (g *Gateway) Stop() {
	_ = g.server.Close()
}

// Addr returns the address the gateway HTTP listener is bound to
func (g *Gateway) Addr() string {
	if g.listener == nil {
		return ""
	}

	return g.listener.Addr().String()
}
