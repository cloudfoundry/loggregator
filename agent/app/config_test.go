package app_test

import (
	"strings"

	"code.cloudfoundry.org/loggregator/agent/app"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Config", func() {
	It("IDN encodes RouterAddrWithAZ", func() {
		c, err := app.Parse(strings.NewReader(`{
                "RouterAddr": "router-addr",
                "RouterAddrWithAZ": "jedinečné.router-addr:1234"
        }`))
		Expect(err).ToNot(HaveOccurred())

		Expect(c.RouterAddrWithAZ).To(Equal("xn--jedinen-hya63a.router-addr:1234"))
	})

	It("strips @ from RouterAddrWithAZ to be DNS compatable", func() {
		c, err := app.Parse(strings.NewReader(`{
                "RouterAddr": "router-addr",
                "RouterAddrWithAZ": "jedi@nečné.router-addr:1234"
        }`))
		Expect(err).ToNot(HaveOccurred())

		Expect(c.RouterAddrWithAZ).ToNot(ContainSubstring("@"))
	})
})
