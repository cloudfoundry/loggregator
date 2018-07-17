package app

import (
	"log"

	envstruct "code.cloudfoundry.org/go-envstruct"
)

type Config struct {
	LogsProviderAddr       string `env:"LOGS_PROVIDER_ADDR,        required, report"`
	LogsProviderCAPath     string `env:"LOGS_PROVIDER_CA_PATH,     required, report"`
	LogsProviderCertPath   string `env:"LOGS_PROVIDER_CERT_PATH,   required, report"`
	LogsProviderKeyPath    string `env:"LOGS_PROVIDER_KEY_PATH,    required, report"`
	LogsProviderCommonName string `env:"LOGS_PROVIDER_COMMON_NAME,           report"`

	GatewayAddr string `env:"GATEWAY_ADDR, report"`
}

func LoadConfig() Config {
	cfg := Config{
		GatewayAddr:            "localhost:8088",
		LogsProviderCommonName: "reverselogproxy",
	}

	if err := envstruct.Load(&cfg); err != nil {
		log.Panicf("failed to load config from environment: %s", err)
	}

	envstruct.WriteReport(&cfg)

	return Config{}
}
