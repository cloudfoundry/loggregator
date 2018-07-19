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

type Gateway struct {
	cfg      Config
	listener net.Listener
	server   *http.Server
}

func NewGateway(cfg Config) *Gateway {
	return &Gateway{
		cfg: cfg,
	}
}

func (g *Gateway) Start(blocking bool) {
	creds, err := plumbing.NewClientCredentials(
		g.cfg.LogsProviderCertPath,
		g.cfg.LogsProviderKeyPath,
		g.cfg.LogsProviderCAPath,
		g.cfg.LogsProviderCommonName,
	)
	if err != nil {
		log.Fatalf("failed to load client TLS config: %s", err)
	}

	log.Println("certs loaded!!!!!!")
	lc := ingress.NewLogClient(creds, g.cfg.LogsProviderAddr)
	log.Println("log client created!!!!!!")
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

func (g *Gateway) Stop() {
	_ = g.server.Close()
}

func (g *Gateway) Addr() string {
	if g.listener == nil {
		return ""
	}

	return g.listener.Addr().String()
}
