package endtoend_test

import (
	"time"

	"code.cloudfoundry.org/loggregator/integration_tests/endtoend"
	"code.cloudfoundry.org/loggregator/testservers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("End to end tests", func() {
	It("sends messages from metron through doppler and traffic controller", func() {
		etcdCleanup, etcdClientURL := testservers.StartTestEtcd()
		defer etcdCleanup()
		dopplerCleanup, dopplerPorts := testservers.StartDoppler(
			testservers.BuildDopplerConfig(etcdClientURL, 0, 0),
		)
		defer dopplerCleanup()
		metronCleanup, metronPorts := testservers.StartMetron(
			testservers.BuildMetronConfig("localhost", dopplerPorts.GRPC),
		)
		defer metronCleanup()
		trafficcontrollerCleanup, tcPorts := testservers.StartTrafficController(
			testservers.BuildTrafficControllerConf(
				etcdClientURL,
				dopplerPorts.GRPC,
				metronPorts.UDP,
			),
		)
		defer trafficcontrollerCleanup()

		const writeRatePerSecond = 10
		metronStreamWriter := endtoend.NewMetronStreamWriter(metronPorts.UDP)
		generator := endtoend.NewLogMessageGenerator("custom-app-id")
		writeStrategy := endtoend.NewConstantWriteStrategy(generator, metronStreamWriter, writeRatePerSecond)

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
