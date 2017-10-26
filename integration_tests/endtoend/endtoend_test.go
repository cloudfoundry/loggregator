package endtoend_test

import (
	"time"

	"code.cloudfoundry.org/loggregator/integration_tests/endtoend"
	"code.cloudfoundry.org/loggregator/testservers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("End to end tests", func() {
	It("sends messages from agent through doppler and traffic controller", func() {
		dopplerCleanup, dopplerPorts := testservers.StartRouter(
			testservers.BuildRouterConfig(0, 0),
		)
		defer dopplerCleanup()
		agentCleanup, agentPorts := testservers.StartAgent(
			testservers.BuildAgentConfig("localhost", dopplerPorts.GRPC),
		)
		defer agentCleanup()
		trafficcontrollerCleanup, tcPorts := testservers.StartTrafficController(
			testservers.BuildTrafficControllerConf(
				dopplerPorts.GRPC,
				agentPorts.UDP,
			),
		)
		defer trafficcontrollerCleanup()

		const writeRatePerSecond = 10
		agentStreamWriter := endtoend.NewAgentStreamWriter(agentPorts.UDP)
		generator := endtoend.NewLogMessageGenerator("custom-app-id")
		writeStrategy := endtoend.NewConstantWriteStrategy(generator, agentStreamWriter, writeRatePerSecond)

		firehoseReader := endtoend.NewFirehoseReader(tcPorts.WS)
		ex := endtoend.NewExperiment(firehoseReader)
		ex.AddWriteStrategy(writeStrategy)

		ex.Warmup()
		go func() {
			defer ex.Stop()
			time.Sleep(2 * time.Second)
		}()
		ex.Start()

		Eventually(firehoseReader.LogMessages).Should(Receive(ContainSubstring("custom-app-id")))
	}, 10)
})
