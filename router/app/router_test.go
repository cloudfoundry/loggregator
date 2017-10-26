package app_test

import (
	"code.cloudfoundry.org/loggregator/router/app"
	"code.cloudfoundry.org/loggregator/testservers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Router", func() {
	Describe("Addrs()", func() {
		It("returns a struct with all the addrs", func() {
			grpc := app.GRPC{
				CAFile:   testservers.Cert("loggregator-ca.crt"),
				CertFile: testservers.Cert("doppler.crt"),
				KeyFile:  testservers.Cert("doppler.key"),
			}

			r := app.NewRouter(grpc)
			r.Start()

			addrs := r.Addrs()

			Expect(addrs.Health).ToNot(Equal(""))
			Expect(addrs.Health).ToNot(Equal("0.0.0.0:0"))
			Expect(addrs.GRPC).ToNot(Equal(""))
			Expect(addrs.GRPC).ToNot(Equal("0.0.0.0:0"))
		})
	})
})
