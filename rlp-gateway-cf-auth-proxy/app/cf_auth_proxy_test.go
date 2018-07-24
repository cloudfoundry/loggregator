package app_test

import (
	"net/http"
	"net/http/httptest"

	"code.cloudfoundry.org/loggregator/rlp-gateway-cf-auth-proxy/app"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("CFAuthProxy", func() {
	It("proxies requests to rlp gateway", func() {
		var called bool
		testServer := httptest.NewServer(
			http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				called = true
			}))

		proxy := app.NewCFAuthProxy(testServer.URL, "127.0.0.1:0")
		proxy.Start()

		resp, err := http.Get("http://" + proxy.Addr())
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(called).To(BeTrue())
	})

	It("delegates to the auth middleware", func() {
		var middlewareCalled bool
		middleware := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			middlewareCalled = true
			w.WriteHeader(http.StatusNotFound)
		})

		proxy := app.NewCFAuthProxy(
			"https://127.0.0.1",
			"127.0.0.1:0",
			app.WithAuthMiddleware(func(http.Handler) http.Handler {
				return middleware
			}),
		)
		proxy.Start()

		resp, err := http.Get("http://" + proxy.Addr())
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
		Expect(middlewareCalled).To(BeTrue())
	})
})
