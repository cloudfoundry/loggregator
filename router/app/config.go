package app

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"time"
)

const HeartbeatInterval = 10 * time.Second

type Agent struct {
	UDPAddress  string
	GRPCAddress string
}

type GRPC struct {
	Port         uint16
	CertFile     string
	KeyFile      string
	CAFile       string
	CipherSuites []string
}

type Config struct {
	MaxRetainedLogMessages       uint32
	MessageDrainBufferSize       uint
	ContainerMetricTTLSeconds    int
	SinkInactivityTimeoutSeconds int

	MetricBatchIntervalMilliseconds uint
	WebsocketHost                   string
	GRPC                            GRPC
	UnmarshallerCount               int

	PPROFPort  uint32
	HealthAddr string
	Agent      Agent
}

func (c *Config) validate() (err error) {
	if c.MaxRetainedLogMessages == 0 {
		return errors.New("Need max number of log messages to retain per application")
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

	return nil
}

func ParseConfig(configFile string) (*Config, error) {
	file, err := os.Open(configFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	b, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}

	return Parse(b)
}

func Parse(confData []byte) (*Config, error) {
	config := &Config{}

	err := json.Unmarshal(confData, config)
	if err != nil {
		return nil, err
	}

	err = config.validate()
	if err != nil {
		return nil, err
	}

	// TODO: These probably belong in the Config literal, above.
	// However, in the interests of not breaking things, we're
	// leaving them for further team discussion.
	if config.MetricBatchIntervalMilliseconds == 0 {
		config.MetricBatchIntervalMilliseconds = 5000
	}

	if config.UnmarshallerCount == 0 {
		config.UnmarshallerCount = 1
	}

	if config.HealthAddr == "" {
		config.HealthAddr = "localhost:14825"
	}

	return config, nil
}
