package app

import (
	envstruct "code.cloudfoundry.org/go-envstruct"
)

type CAPI struct {
	Addr       string `env:"CAPI_ADDR,        required, report"`
	CertPath   string `env:"CAPI_CERT_PATH,   required, report"`
	KeyPath    string `env:"CAPI_KEY_PATH,    required, report"`
	CAPath     string `env:"CAPI_CA_PATH,     required, report"`
	CommonName string `env:"CAPI_COMMON_NAME, required, report"`

	ExternalAddr string `env:"CAPI_ADDR_EXTERNAL, required, report"`
}

type UAA struct {
	ClientID     string `env:"UAA_CLIENT_ID,     required"`
	ClientSecret string `env:"UAA_CLIENT_SECRET, required"`
	Addr         string `env:"UAA_ADDR,          required, report"`
	CAPath       string `env:"UAA_CA_PATH,       required, report"`
}

type Config struct {
	RLPGatewayAddr string `env:"RLP_GATEWAY_ADDR, required, report"`
	Addr           string `env:"ADDR, required, report"`
	HealthAddr     string `env:"HEALTH_ADDR, report"`
	SkipCertVerify bool   `env:"SKIP_CERT_VERIFY, report"`

	CAPI CAPI
	UAA  UAA
}

func LoadConfig() (*Config, error) {
	cfg := Config{
		SkipCertVerify: false,
		Addr:           ":8083",
		HealthAddr:     "localhost:6065",
		RLPGatewayAddr: "localhost:8088",
	}

	err := envstruct.Load(&cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}
