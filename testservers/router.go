package testservers

import (
	"fmt"
	"os"
	"os/exec"

	"code.cloudfoundry.org/loggregator/router/app"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

func BuildRouterConfig(metronUDPPort, metronGRPCPort int) app.Config {
	return app.Config{
		GRPC: app.GRPC{
			CertFile: Cert("doppler.crt"),
			KeyFile:  Cert("doppler.key"),
			CAFile:   Cert("loggregator-ca.crt"),
		},
		HealthAddr: "localhost:0",

		MetronConfig: app.MetronConfig{
			UDPAddress:  fmt.Sprintf("127.0.0.1:%d", metronUDPPort),
			GRPCAddress: fmt.Sprintf("127.0.0.1:%d", metronGRPCPort),
		},

		MetricBatchIntervalMilliseconds: 10,
		ContainerMetricTTLSeconds:       120,
		MaxRetainedLogMessages:          10,
		MessageDrainBufferSize:          100,
		SinkInactivityTimeoutSeconds:    120,
		UnmarshallerCount:               5,
	}
}

type RouterPorts struct {
	GRPC   int
	PProf  int
	Health int
}

func StartRouter(conf app.Config) (cleanup func(), rp RouterPorts) {
	By("making sure router was built")
	routerPath := os.Getenv("ROUTER_BUILD_PATH")
	Expect(routerPath).ToNot(BeEmpty())

	filename, err := writeConfigToFile("router-config", conf)
	Expect(err).ToNot(HaveOccurred())

	By("starting router")
	routerCommand := exec.Command(routerPath, "--config", filename)

	routerSession, err := gexec.Start(
		routerCommand,
		gexec.NewPrefixedWriter(color("o", "router", green, blue), GinkgoWriter),
		gexec.NewPrefixedWriter(color("e", "router", red, blue), GinkgoWriter),
	)
	Expect(err).ToNot(HaveOccurred())

	By("waiting for router to listen")
	rp.GRPC = waitForPortBinding("grpc", routerSession.Err)
	rp.PProf = waitForPortBinding("pprof", routerSession.Err)
	rp.Health = waitForPortBinding("health", routerSession.Err)

	cleanup = func() {
		os.Remove(filename)
		routerSession.Kill().Wait()
	}

	return cleanup, rp
}
