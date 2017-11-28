package app

import (
	"fmt"
	"strings"

	envstruct "code.cloudfoundry.org/go-envstruct"
	"golang.org/x/net/idna"
)

// GRPC stores the configuration for the router as a server using a PORT
// with mTLS certs and as a client.
type GRPC struct {
	Port         uint16   `env:"AGENT_PORT"`
	CAFile       string   `env:"AGENT_CA_FILE"`
	CertFile     string   `env:"AGENT_CERT_FILE"`
	KeyFile      string   `env:"AGENT_KEY_FILE"`
	CipherSuites []string `env:"AGENT_CIPHER_SUITES"`
}

// Config stores all configurations options for the Agent.
type Config struct {
	Deployment                      string            `env:"AGENT_DEPLOYMENT"`
	Zone                            string            `env:"AGENT_ZONE"`
	Job                             string            `env:"AGENT_JOB"`
	Index                           string            `env:"AGENT_INDEX"`
	IP                              string            `env:"AGENT_IP"`
	Tags                            map[string]string `env:"AGENT_TAGS"`
	DisableUDP                      bool              `env:"AGENT_DISABLE_UDP"`
	IncomingUDPPort                 int               `env:"AGENT_INCOMING_UDP_PORT"`
	HealthEndpointPort              uint              `env:"AGENT_HEALTH_ENDPOINT_PORT"`
	MetricBatchIntervalMilliseconds uint              `env:"AGENT_METRIC_BATCH_INTERVAL_MILLISECONDS"`
	PProfPort                       uint32            `env:"AGENT_PPROF_PORT"`
	RouterAddr                      string            `env:"ROUTER_ADDR"`
	RouterAddrWithAZ                string            `env:"ROUTER_ADDR_WITH_AZ"`
	GRPC                            GRPC
}

// LoadConfig reads from the environment to create a Config.
func LoadConfig() (*Config, error) {
	config := Config{
		MetricBatchIntervalMilliseconds: 60000,
		IncomingUDPPort:                 3457,
		HealthEndpointPort:              14824,
		GRPC: GRPC{
			Port: 3458,
		},
	}
	err := envstruct.Load(&config)
	if err != nil {
		return nil, err
	}

	if config.RouterAddr == "" {
		return nil, fmt.Errorf("RouterAddr is required")
	}

	config.RouterAddrWithAZ, err = idna.ToASCII(config.RouterAddrWithAZ)
	if err != nil {
		return nil, err
	}
	config.RouterAddrWithAZ = strings.Replace(config.RouterAddrWithAZ, "@", "-", -1)

	return &config, nil
}
