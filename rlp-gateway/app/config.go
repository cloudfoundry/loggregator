package app

import (
	"log"

	envstruct "code.cloudfoundry.org/go-envstruct"
)

// Config holds the configuration for the RLP Gateway
type Config struct {
	LogsProviderAddr           string `env:"LOGS_PROVIDER_ADDR,             required, report"`
	LogsProviderCAPath         string `env:"LOGS_PROVIDER_CA_PATH,          required, report"`
	LogsProviderClientCertPath string `env:"LOGS_PROVIDER_CLIENT_CERT_PATH, required, report"`
	LogsProviderClientKeyPath  string `env:"LOGS_PROVIDER_CLIENT_KEY_PATH,  required, report"`
	LogsProviderCommonName     string `env:"LOGS_PROVIDER_COMMON_NAME, report"`

	GatewayAddr string `env:"GATEWAY_ADDR, report"`
	PProfPort   uint32 `env:"PPROF_PORT"`
}

// LoadConfig will load and return the config from the current environment. If
// this fails this function will fatally log.
func LoadConfig() Config {
	cfg := Config{
		GatewayAddr:            "localhost:8088",
		LogsProviderCommonName: "reverselogproxy",
	}

	if err := envstruct.Load(&cfg); err != nil {
		log.Fatalf("failed to load config from environment: %s", err)
	}

	envstruct.WriteReport(&cfg)

	return cfg
}
