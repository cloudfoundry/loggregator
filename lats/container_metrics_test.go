package lats_test

import (
	"github.com/cloudfoundry/sonde-go/events"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Container Metrics Endpoint", func() {
	It("can receive container metrics", func() {
		envelope := createContainerMetric("test-id")
		EmitToMetronV1(envelope)

		f := func() []*events.ContainerMetric {
			return RequestContainerMetrics("test-id")
		}
		Eventually(f).Should(ContainElement(envelope.ContainerMetric))
	})
})
