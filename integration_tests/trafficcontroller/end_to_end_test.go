package trafficcontroller_test

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator/testservers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry/noaa/consumer"
	"github.com/cloudfoundry/sonde-go/events"
)

var _ = Describe("TrafficController for v1 messages", func() {
	var (
		logCache     *stubGrpcLogCache
		fakeDoppler  *FakeDoppler
		tcWSEndpoint string
		wsPort       int
	)

	Context("without a configured log cache", func() {
		BeforeEach(func() {
			fakeDoppler = NewFakeDoppler()
			err := fakeDoppler.Start()
			Expect(err).ToNot(HaveOccurred())

			cfg := testservers.BuildTrafficControllerConfWithoutLogCache(fakeDoppler.Addr(), 37474)

			var tcPorts testservers.TrafficControllerPorts
			tcCleanupFunc, tcPorts = testservers.StartTrafficController(cfg)

			wsPort = tcPorts.WS

			tcHTTPEndpoint := fmt.Sprintf("http://%s:%d", localIPAddress, tcPorts.WS)
			tcWSEndpoint = fmt.Sprintf("ws://%s:%d", localIPAddress, tcPorts.WS)

			// wait for TC
			Eventually(func() error {
				resp, err := http.Get(tcHTTPEndpoint)
				if err == nil {
					resp.Body.Close()
				}
				return err
			}, 10).Should(Succeed())
		})

		AfterEach(func(done Done) {
			defer close(done)
			fakeDoppler.Stop()
		}, 30)

		Describe("LogCache API Paths", func() {
			Context("Recent", func() {
				It("returns a helpful error message", func() {
					client := consumer.New(tcWSEndpoint, &tls.Config{}, nil)

					logMessages, err := client.RecentLogs("efe5c422-e8a7-42c2-a52b-98bffd8d6a07", "bearer iAmAnAdmin")

					Expect(err).ToNot(HaveOccurred())
					Expect(logMessages).To(HaveLen(1))
					Expect(string(logMessages[0].GetMessage())).To(Equal("recent log endpoint requires a log cache. please talk to you operator"))
				})
			})

			Context("ContainerMetrics", func() {
				It("returns an empty result", func() {
					client := consumer.New(tcWSEndpoint, &tls.Config{}, nil)

					metrics, err := client.ContainerMetrics("efe5c422-e8a7-42c2-a52b-98bffd8d6a07", "bearer iAmAnAdmin")

					Expect(err).ToNot(HaveOccurred())
					Expect(metrics).To(HaveLen(1))
				})
			})
		})
	})

	Context("with a configured log cache", func() {
		BeforeEach(func() {
			fakeDoppler = NewFakeDoppler()
			err := fakeDoppler.Start()
			Expect(err).ToNot(HaveOccurred())

			logCache = newStubGrpcLogCache()
			cfg := testservers.BuildTrafficControllerConf(fakeDoppler.Addr(), 37474, logCache.addr())

			var tcPorts testservers.TrafficControllerPorts
			tcCleanupFunc, tcPorts = testservers.StartTrafficController(cfg)

			wsPort = tcPorts.WS

			tcHTTPEndpoint := fmt.Sprintf("http://%s:%d", localIPAddress, tcPorts.WS)
			tcWSEndpoint = fmt.Sprintf("ws://%s:%d", localIPAddress, tcPorts.WS)

			// wait for TC
			Eventually(func() error {
				resp, err := http.Get(tcHTTPEndpoint)
				if err == nil {
					resp.Body.Close()
				}
				return err
			}, 10).Should(Succeed())
		})

		AfterEach(func(done Done) {
			defer close(done)
			fakeDoppler.Stop()
			logCache.stop()
		}, 30)

		Describe("Loggregator Router Paths", func() {
			Context("Streaming", func() {
				var (
					client   *consumer.Consumer
					messages <-chan *events.Envelope
					errors   <-chan error
				)

				BeforeEach(func() {
					client = consumer.New(tcWSEndpoint, &tls.Config{}, nil)
					messages, errors = client.StreamWithoutReconnect(APP_ID, AUTH_TOKEN)
				})

				It("passes messages through", func() {
					var grpcRequest *loggregator_v2.EgressBatchRequest
					Eventually(fakeDoppler.EgressBatchRequests, 10).Should(Receive(&grpcRequest))
					Expect(grpcRequest.Selectors).To(HaveLen(2))

					selector := grpcRequest.Selectors[0]
					Expect(selector.SourceId).To(Equal(APP_ID))
					Expect(selector.GetLog()).ToNot(BeNil())

					selector = grpcRequest.Selectors[1]
					Expect(selector.SourceId).To(Equal(APP_ID))
					Expect(selector.GetGauge()).ToNot(BeNil())

					fakeDoppler.SendV2LogMessage(APP_ID, "Hello through NOAA")

					var receivedEnvelope *events.Envelope
					Eventually(messages).Should(Receive(&receivedEnvelope))
					Consistently(errors).ShouldNot(Receive())

					receivedMessage := receivedEnvelope.GetLogMessage()
					Expect(receivedMessage.GetMessage()).To(BeEquivalentTo("Hello through NOAA"))
					Expect(receivedMessage.GetAppId()).To(Equal(APP_ID))

					client.Close()
				})

				It("closes the upstream websocket connection when done", func() {
					var server loggregator_v2.Egress_BatchedReceiverServer
					Eventually(fakeDoppler.EgressBatchStreams, 10).Should(Receive(&server))

					client.Close()

					Eventually(server.Context().Done()).Should(BeClosed())
				})
			})

			Context("Firehose", func() {
				var (
					messages <-chan *events.Envelope
					errors   <-chan error
				)

				It("passes messages through for every app for uaa admins", func() {
					client := consumer.New(tcWSEndpoint, &tls.Config{}, nil)
					defer client.Close()
					messages, errors = client.FirehoseWithoutReconnect(SUBSCRIPTION_ID, AUTH_TOKEN)

					var grpcRequest *loggregator_v2.EgressBatchRequest
					Eventually(fakeDoppler.EgressBatchRequests, 10).Should(Receive(&grpcRequest))
					Expect(grpcRequest.ShardId).To(Equal(SUBSCRIPTION_ID))

					fakeDoppler.SendV2LogMessage(APP_ID, "Hello through NOAA")

					var receivedEnvelope *events.Envelope
					Eventually(messages).Should(Receive(&receivedEnvelope))
					Consistently(errors).ShouldNot(Receive())

					Expect(receivedEnvelope.GetEventType()).To(Equal(events.Envelope_LogMessage))
					receivedMessage := receivedEnvelope.GetLogMessage()
					Expect(receivedMessage.GetMessage()).To(BeEquivalentTo("Hello through NOAA"))
					Expect(receivedMessage.GetAppId()).To(Equal(APP_ID))
				})
			})
		})

		Describe("LogCache API Paths", func() {
			Context("Recent", func() {
				It("returns a multi-part HTTP response with all recent messages", func() {
					client := consumer.New(tcWSEndpoint, &tls.Config{}, nil)

					Eventually(func() int {
						messages, err := client.RecentLogs("efe5c422-e8a7-42c2-a52b-98bffd8d6a07", "bearer iAmAnAdmin")
						Expect(err).NotTo(HaveOccurred())

						if len(logCache.requests()) > 0 {
							Expect(logCache.requests()[0].SourceId).To(Equal("efe5c422-e8a7-42c2-a52b-98bffd8d6a07"))
						}

						return len(messages)
					}, 5).Should(Equal(2))
				})
			})

			Context("ContainerMetrics", func() {
				It("returns a multi-part HTTP response with the most recent container metrics for all instances for a given app", func() {
					client := consumer.New(tcWSEndpoint, &tls.Config{}, nil)

					Eventually(func() bool {
						messages, err := client.ContainerMetrics("efe5c422-e8a7-42c2-a52b-98bffd8d6a07", "bearer iAmAnAdmin")
						Expect(err).NotTo(HaveOccurred())

						return len(messages) > 0
					}, 5).Should(BeTrue())
				})
			})
		})

		Context("SetCookie", func() {
			It("sets the desired cookie on the response", func() {
				response, err := http.PostForm(
					fmt.Sprintf("http://%s:%d/set-cookie",
						localIPAddress,
						wsPort,
					),
					url.Values{
						"CookieName":  {"authorization"},
						"CookieValue": {url.QueryEscape("bearer iAmAnAdmin")},
					},
				)
				Expect(err).NotTo(HaveOccurred())

				Expect(response.Cookies()).NotTo(BeNil())
				Expect(response.Cookies()).To(HaveLen(1))
				cookie := response.Cookies()[0]
				Expect(cookie.Domain).To(Equal("doppler.vcap.me"))
				Expect(cookie.Name).To(Equal("authorization"))
				Expect(cookie.Value).To(Equal("bearer+iAmAnAdmin"))
				Expect(cookie.Secure).To(BeTrue())
			})
		})
	})
})
