package app

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"
)

// Agent holds configuration for communication to a logging/metric agent.
type Agent struct {
	UDPAddress  string
	GRPCAddress string
}

// GRPC holds TLS configuration for gRPC communcation to router and agent.
// Port is the Port to dial for communcation with router.
type GRPC struct {
	Port     uint16
	CAFile   string
	CertFile string
	KeyFile  string
}

// CCTLSClientConfig holds TLS cofiguration for communication with cloud
// controller.
type CCTLSClientConfig struct {
	CertFile   string
	KeyFile    string
	CAFile     string
	ServerName string
}

// Config holds all Configuration options for trafficcontroller.
type Config struct {
	IP                    string
	ApiHost               string
	CCTLSClientConfig     CCTLSClientConfig
	RouterPort            uint32
	RouterAddrs           []string
	OutgoingDropsondePort uint32
	Agent                 Agent
	GRPC                  GRPC
	SystemDomain          string
	SkipCertVerify        bool
	UaaHost               string
	UaaClient             string
	UaaClientSecret       string
	UaaCACert             string
	SecurityEventLog      string
	PPROFPort             uint32
	MetricEmitterInterval string
	MetricEmitterDuration time.Duration `json:"-"`
	HealthAddr            string
}

// ParseConfig reads ands and parses the given filepath to a JSON file.
func ParseConfig(configFile string) (*Config, error) {
	file, err := os.Open(configFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return Parse(file)
}

// Parse returns a Config struct that has been unmarshalled from the given
// io.Reader. Before the Config struct is returned, defaults are set and
// validation is performed.
func Parse(r io.Reader) (*Config, error) {
	config := &Config{}

	err := json.NewDecoder(r).Decode(config)
	if err != nil {
		return nil, err
	}

	config.setDefaults()

	err = config.validate()
	if err != nil {
		return nil, err
	}
	return config, nil
}

func (c *Config) setDefaults() {
	if c.GRPC.Port == 0 {
		c.GRPC.Port = 8082
	}

	duration, err := time.ParseDuration(c.MetricEmitterInterval)
	if err != nil {
		c.MetricEmitterDuration = time.Minute
	} else {
		c.MetricEmitterDuration = duration
	}
	if len(c.HealthAddr) == 0 {
		c.HealthAddr = "localhost:14825"
	}
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
