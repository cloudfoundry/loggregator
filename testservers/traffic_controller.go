package testservers

import (
	"fmt"
	"os"
	"os/exec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	tcConf "code.cloudfoundry.org/loggregator/trafficcontroller/app"
)

func BuildTrafficControllerConf(dopplerGRPCPort, agentPort int) tcConf.Config {
	return tcConf.Config{
		IP:          "127.0.0.1",
		RouterAddrs: []string{fmt.Sprintf("127.0.0.1:%d", dopplerGRPCPort)},

		GRPC: tcConf.GRPC{
			Port:     uint16(dopplerGRPCPort),
			CertFile: Cert("trafficcontroller.crt"),
			KeyFile:  Cert("trafficcontroller.key"),
			CAFile:   Cert("loggregator-ca.crt"),
		},
		Agent: tcConf.Agent{
			UDPAddress: fmt.Sprintf("localhost:%d", agentPort),
		},
		HealthAddr: "localhost:0",

		SystemDomain:   "vcap.me",
		SkipCertVerify: true,

		ApiHost:         "http://127.0.0.1:65530",
		UaaHost:         "http://127.0.0.1:65531",
		UaaCACert:       Cert("loggregator-ca.crt"),
		UaaClient:       "bob",
		UaaClientSecret: "yourUncle",
		CCTLSClientConfig: tcConf.CCTLSClientConfig{
			CertFile:   Cert("trafficcontroller.crt"),
			KeyFile:    Cert("trafficcontroller.key"),
			CAFile:     Cert("loggregator-ca.crt"),
			ServerName: "cloud-controller",
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

	filename, err := writeConfigToFile("trafficcontroller-config", conf)
	Expect(err).ToNot(HaveOccurred())

	By("starting trafficcontroller")
	tcCommand := exec.Command(tcPath, "--disableAccessControl", "--config", filename)
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
		os.Remove(filename)
		tcSession.Kill().Wait()
	}
	return cleanup, tp
}
