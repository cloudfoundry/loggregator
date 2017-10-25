package testservers

import (
	"fmt"
	"os"
	"os/exec"

	envstruct "code.cloudfoundry.org/go-envstruct"
	"code.cloudfoundry.org/loggregator/agent/app"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

func BuildAgentConfig(dopplerURI string, dopplerGRPCPort int) app.Config {
	return app.Config{
		Index: jobIndex,
		Job:   jobName,
		Zone:  availabilityZone,

		Tags: map[string]string{
			"auto-tag-1": "auto-tag-value-1",
			"auto-tag-2": "auto-tag-value-2",
		},

		Deployment: "deployment",

		RouterAddr:       fmt.Sprintf("%s:%d", dopplerURI, dopplerGRPCPort),
		RouterAddrWithAZ: fmt.Sprintf("%s.%s:%d", availabilityZone, dopplerURI, dopplerGRPCPort),

		GRPC: app.GRPC{
			CertFile: Cert("metron.crt"),
			KeyFile:  Cert("metron.key"),
			CAFile:   Cert("loggregator-ca.crt"),
		},

		MetricBatchIntervalMilliseconds:  5000,
		RuntimeStatsIntervalMilliseconds: 10,
	}
}

type AgentPorts struct {
	GRPC   int
	UDP    int
	Health int
	PProf  int
}

func StartAgent(conf app.Config) (cleanup func(), mp AgentPorts) {
	By("making sure agent was built")
	agentPath := os.Getenv("AGENT_BUILD_PATH")
	Expect(agentPath).ToNot(BeEmpty())

	By("starting agent")
	agentCommand := exec.Command(agentPath)
	agentCommand.Env = envstruct.ToEnv(&conf)
	agentSession, err := gexec.Start(
		agentCommand,
		gexec.NewPrefixedWriter(color("o", "agent", green, magenta), GinkgoWriter),
		gexec.NewPrefixedWriter(color("e", "agent", red, magenta), GinkgoWriter),
	)
	Expect(err).ToNot(HaveOccurred())

	By("waiting for agent to listen")
	mp.GRPC = waitForPortBinding("grpc", agentSession.Err)
	mp.UDP = waitForPortBinding("udp", agentSession.Err)
	mp.Health = waitForPortBinding("health", agentSession.Err)
	mp.PProf = waitForPortBinding("pprof", agentSession.Err)

	cleanup = func() {
		agentSession.Kill().Wait()
	}
	return cleanup, mp
}
