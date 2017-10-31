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

		firehoseReader := endtoend.NewFirehoseReader(tcPorts.WS)

		go func() {
			agentStreamWriter := endtoend.NewAgentStreamWriter(agentPorts.UDP)
			generator := endtoend.NewLogMessageGenerator("custom-app-id")
			for range time.Tick(time.Millisecond) {
				agentStreamWriter.Write(generator.Generate())
				firehoseReader.Read()
			}
		}()

		Eventually(firehoseReader.LogMessageAppIDs, 5).Should(Receive(Equal("custom-app-id")))
	}, 10)
})
