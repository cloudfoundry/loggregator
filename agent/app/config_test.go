package app_test

import (
	"os"

	"code.cloudfoundry.org/loggregator/agent/app"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Config", func() {
	It("IDN encodes RouterAddrWithAZ", func() {
		os.Setenv("ROUTER_ADDR", "router-addr")
		os.Setenv("ROUTER_ADDR_WITH_AZ", "jedinečné.router-addr:1234")

		c, err := app.LoadConfig()

		Expect(err).ToNot(HaveOccurred())
		Expect(c.RouterAddrWithAZ).To(Equal("xn--jedinen-hya63a.router-addr:1234"))
	})

	It("strips @ from RouterAddrWithAZ to be DNS compatable", func() {
		os.Setenv("ROUTER_ADDR", "router-addr")
		os.Setenv("ROUTER_ADDR_WITH_AZ", "jedi@nečné.router-addr:1234")

		c, err := app.LoadConfig()

		Expect(err).ToNot(HaveOccurred())
		Expect(c.RouterAddrWithAZ).ToNot(ContainSubstring("@"))
	})
})
