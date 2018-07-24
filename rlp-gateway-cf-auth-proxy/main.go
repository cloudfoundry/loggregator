package main

import (
	"expvar"
	"io/ioutil"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"

	"crypto/x509"

	"code.cloudfoundry.org/go-envstruct"
	"code.cloudfoundry.org/loggregator/plumbing"
	"code.cloudfoundry.org/loggregator/rlp-gateway-cf-auth-proxy/app"
	"code.cloudfoundry.org/loggregator/rlp-gateway-cf-auth-proxy/internal/auth"
	"code.cloudfoundry.org/loggregator/rlp-gateway-cf-auth-proxy/internal/metrics"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	log := log.New(os.Stderr, "", log.LstdFlags)
	log.Print("Starting RLP Gateway CF Auth Reverse Proxy...")
	defer log.Print("Closing RLP Gateway CF Auth Reverse Proxy.")

	cfg, err := app.LoadConfig()
	if err != nil {
		log.Fatalf("failed to load config: %s", err)
	}
	envstruct.WriteReport(cfg)

	metrics := metrics.New(expvar.NewMap("CFAuthProxy"))

	uaaClient := auth.NewUAAClient(
		cfg.UAA.Addr,
		cfg.UAA.ClientID,
		cfg.UAA.ClientSecret,
		buildUAAClient(cfg),
		metrics,
		log,
	)

	capiClient := auth.NewCAPIClient(
		cfg.CAPI.Addr,
		cfg.CAPI.ExternalAddr,
		buildCAPIClient(cfg),
		metrics,
		log,
	)

	// logProviderClient := web.ReadHandler(lp)

	middlewareProvider := auth.NewCFAuthMiddlewareProvider(
		uaaClient,
		capiClient,
	)

	proxy := app.NewCFAuthProxy(
		cfg.RLPGatewayAddr,
		cfg.Addr,
		app.WithAuthMiddleware(middlewareProvider.Middleware),
	)
	proxy.Start()

	// health endpoints (pprof and expvar)
	log.Printf("Health: %s", http.ListenAndServe(cfg.HealthAddr, nil))
}

func buildUAAClient(cfg *app.Config) *http.Client {
	tlsConfig := plumbing.NewTLSConfig()
	tlsConfig.InsecureSkipVerify = cfg.SkipCertVerify
	tlsConfig.RootCAs = loadUaaCA(cfg.UAA.CAPath)

	transport := &http.Transport{
		TLSHandshakeTimeout: 10 * time.Second,
		TLSClientConfig:     tlsConfig,
		DisableKeepAlives:   true,
	}

	return &http.Client{
		Timeout:   20 * time.Second,
		Transport: transport,
	}
}

func buildCAPIClient(cfg *app.Config) *http.Client {
	tlsConfig, err := plumbing.NewClientMutualTLSConfig(
		cfg.CAPI.CertPath,
		cfg.CAPI.KeyPath,
		cfg.CAPI.CAPath,
		cfg.CAPI.CommonName,
	)
	if err != nil {
		log.Fatalf("unable to create CAPI HTTP Client: %s", err)
	}

	tlsConfig.InsecureSkipVerify = cfg.SkipCertVerify
	transport := &http.Transport{
		TLSHandshakeTimeout: 10 * time.Second,
		TLSClientConfig:     tlsConfig,
		DisableKeepAlives:   true,
	}

	return &http.Client{
		Timeout:   20 * time.Second,
		Transport: transport,
	}
}

func loadUaaCA(uaaCertPath string) *x509.CertPool {
	caCert, err := ioutil.ReadFile(uaaCertPath)
	if err != nil {
		log.Fatalf("failed to read UAA CA certificate: %s", err)
	}

	certPool := x509.NewCertPool()
	ok := certPool.AppendCertsFromPEM(caCert)
	if !ok {
		log.Fatal("failed to parse UAA CA certificate.")
	}

	return certPool
}
