package app

import (
	"errors"
	"time"

	envstruct "code.cloudfoundry.org/go-envstruct"
)

// Agent stores configuration for communication to a logging/metric agent.
type Agent struct {
	UDPAddress  string `env:"AGENT_UDP_ADDRESS"`
	GRPCAddress string `env:"AGENT_GRPC_ADDRESS"`
}

// GRPC stores TLS configuration for gRPC communcation to router and agent.
type GRPC struct {
	CAFile   string `env:"ROUTER_CA_FILE"`
	CertFile string `env:"ROUTER_CERT_FILE"`
	KeyFile  string `env:"ROUTER_KEY_FILE"`
}

// LogCacheTLSConfig stores TLS configuration for gRPC communcation to router and agent.
type LogCacheTLSConfig struct {
	CAFile     string `env:"LOG_CACHE_CA_FILE"`
	CertFile   string `env:"LOG_CACHE_CERT_FILE"`
	KeyFile    string `env:"LOG_CACHE_KEY_FILE"`
	ServerName string `env:"LOG_CACHE_SERVER_NAME"`
}

// CCTLSClientConfig stores TLS cofiguration for communication with cloud
// controller.
type CCTLSClientConfig struct {
	CertFile   string `env:"CC_CERT_FILE"`
	KeyFile    string `env:"CC_KEY_FILE"`
	CAFile     string `env:"CC_CA_FILE"`
	ServerName string `env:"CC_SERVER_NAME"`
}

// Config stores all Configuration options for trafficcontroller.
type Config struct {
	IP                    string        `env:"TRAFFIC_CONTROLLER_IP"`
	ApiHost               string        `env:"TRAFFIC_CONTROLLER_API_HOST"`
	OutgoingDropsondePort uint32        `env:"TRAFFIC_CONTROLLER_OUTGOING_DROPSONDE_PORT"`
	SystemDomain          string        `env:"TRAFFIC_CONTROLLER_SYSTEM_DOMAIN"`
	SkipCertVerify        bool          `env:"TRAFFIC_CONTROLLER_SKIP_CERT_VERIFY"`
	UaaHost               string        `env:"TRAFFIC_CONTROLLER_UAA_HOST"`
	UaaClient             string        `env:"TRAFFIC_CONTROLLER_UAA_CLIENT"`
	UaaClientSecret       string        `env:"TRAFFIC_CONTROLLER_UAA_CLIENT_SECRET"`
	UaaCACert             string        `env:"TRAFFIC_CONTROLLER_UAA_CA_CERT"`
	SecurityEventLog      string        `env:"TRAFFIC_CONTROLLER_SECURITY_EVENT_LOG"`
	PProfPort             uint32        `env:"TRAFFIC_CONTROLLER_PPROF_PORT"`
	MetricEmitterInterval time.Duration `env:"TRAFFIC_CONTROLLER_METRIC_EMITTER_INTERVAL"`
	HealthAddr            string        `env:"TRAFFIC_CONTROLLER_HEALTH_ADDR"`
	DisableAccessControl  bool          `env:"TRAFFIC_CONTROLLER_DISABLE_ACCESS_CONTROL"`
	RouterAddrs           []string      `env:"ROUTER_ADDRS"`
	LogCacheAddr          string        `env:"LOG_CACHE_ADDR"`

	CCTLSClientConfig CCTLSClientConfig
	Agent             Agent
	GRPC              GRPC
	LogCacheTLSConfig LogCacheTLSConfig
}

// LoadConfig reads from the environment to create a Config.
func LoadConfig() (*Config, error) {
	config := Config{
		MetricEmitterInterval: time.Minute,
		HealthAddr:            "localhost:14825",
		LogCacheTLSConfig: LogCacheTLSConfig{
			ServerName: "log_cache",
		},
	}

	err := envstruct.Load(&config)
	if err != nil {
		return nil, err
	}

	err = config.validate()
	if err != nil {
		return nil, err
	}

	return &config, nil
}

func (c *Config) validate() error {
	if c.SystemDomain == "" {
		return errors.New("Need system domain in order to create the proxies")
	}

	if c.IP == "" {
		return errors.New("Need IP address for access logging")
	}

	if len(c.GRPC.CAFile) == 0 {
		return errors.New("invalid router config, no GRPC.CAFile provided")
	}

	if len(c.GRPC.CertFile) == 0 {
		return errors.New("invalid router config, no GRPC.CertFile provided")
	}

	if len(c.GRPC.KeyFile) == 0 {
		return errors.New("invalid router config, no GRPC.KeyFile provided")
	}

	if c.UaaClientSecret == "" {
		return errors.New("missing UAA client secret")
	}

	if c.UaaHost != "" && c.UaaCACert == "" {
		return errors.New("missing UAA CA certificate")
	}

	return nil
}
