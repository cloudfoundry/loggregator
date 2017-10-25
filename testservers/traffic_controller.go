package testservers

import (
	"fmt"
	"os"
	"os/exec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	envstruct "code.cloudfoundry.org/go-envstruct"
	tcConf "code.cloudfoundry.org/loggregator/trafficcontroller/app"
)

func BuildTrafficControllerConf(dopplerGRPCPort, agentPort int) tcConf.Config {
	return tcConf.Config{
		IP:                    "127.0.0.1",
		RouterAddrs:           []string{fmt.Sprintf("127.0.0.1:%d", dopplerGRPCPort)},
		HealthAddr:            "localhost:11111",
		SystemDomain:          "vcap.me",
		SkipCertVerify:        true,
		ApiHost:               "http://127.0.0.1:65530",
		UaaHost:               "http://127.0.0.1:65531",
		UaaCACert:             Cert("loggregator-ca.crt"),
		UaaClient:             "bob",
		UaaClientSecret:       "yourUncle",
		DisableAccessControl:  true,
		OutgoingDropsondePort: 4566,
		CCTLSClientConfig: tcConf.CCTLSClientConfig{
			CertFile:   Cert("trafficcontroller.crt"),
			KeyFile:    Cert("trafficcontroller.key"),
			CAFile:     Cert("loggregator-ca.crt"),
			ServerName: "cloud-controller",
		},
		Agent: tcConf.Agent{
			UDPAddress: fmt.Sprintf("localhost:%d", agentPort),
		},
		GRPC: tcConf.GRPC{
			CertFile: Cert("trafficcontroller.crt"),
			KeyFile:  Cert("trafficcontroller.key"),
			CAFile:   Cert("loggregator-ca.crt"),
		},
	}
}

type TrafficControllerPorts struct {
	WS     int
	Health int
	PProf  int
}

func StartTrafficController(conf tcConf.Config) (cleanup func(), tp TrafficControllerPorts) {
	By("making sure trafficcontroller was built")
	tcPath := os.Getenv("TRAFFIC_CONTROLLER_BUILD_PATH")
	Expect(tcPath).ToNot(BeEmpty())

	By("starting trafficcontroller")
	tcCommand := exec.Command(tcPath)
	tcCommand.Env = envstruct.ToEnv(&conf)
	tcSession, err := gexec.Start(
		tcCommand,
		gexec.NewPrefixedWriter(color("o", "tc", green, cyan), GinkgoWriter),
		gexec.NewPrefixedWriter(color("e", "tc", red, cyan), GinkgoWriter),
	)
	Expect(err).ToNot(HaveOccurred())

	By("waiting for trafficcontroller to listen")
	tp.WS = waitForPortBinding("ws", tcSession.Err)
	tp.Health = waitForPortBinding("health", tcSession.Err)
	tp.PProf = waitForPortBinding("pprof", tcSession.Err)

	cleanup = func() {
		tcSession.Kill().Wait()
	}
	return cleanup, tp
}
