package app_test

import (
	"context"
	"time"

	"code.cloudfoundry.org/loggregator/plumbing"
	"code.cloudfoundry.org/loggregator/plumbing/v2"
	"code.cloudfoundry.org/loggregator/router/app"
	"code.cloudfoundry.org/loggregator/testservers"
	"google.golang.org/grpc"

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
			defer r.Stop()

			addrs := r.Addrs()

			Expect(addrs.Health).ToNot(Equal(""))
			Expect(addrs.Health).ToNot(Equal("0.0.0.0:0"))
			Expect(addrs.GRPC).ToNot(Equal(""))
			Expect(addrs.GRPC).ToNot(Equal("0.0.0.0:0"))
		})
	})

	Describe("Selectors", func() {
		Context("when no selectors are given", func() {
			It("should not egress any envelopes", func() {
				grpc := app.GRPC{
					CAFile:   testservers.Cert("loggregator-ca.crt"),
					CertFile: testservers.Cert("doppler.crt"),
					KeyFile:  testservers.Cert("doppler.key"),
				}

				r := app.NewRouter(grpc)
				r.Start()
				defer r.Stop()

				addrs := r.Addrs()

				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				ingressClient := createRouterIngressClient(addrs.GRPC, grpc)
				sender, err := ingressClient.BatchSender(ctx)
				Expect(err).ToNot(HaveOccurred())

				go func() {
					defer GinkgoRecover()
					ticker := time.NewTicker(50 * time.Millisecond)
					for {
						select {
						case <-ctx.Done():
							return
						case <-ticker.C:
							err := sender.Send(&loggregator_v2.EnvelopeBatch{
								Batch: []*loggregator_v2.Envelope{
									genericLogEnvelope(),
								},
							})
							if err != nil {
								println(err.Error())
							}
						}
					}
				}()

				client := createRouterV2EgressClient(addrs.GRPC, grpc)

				rx, err := client.BatchedReceiver(context.Background(), &loggregator_v2.EgressBatchRequest{
					Selectors: nil,
				})

				Expect(err).ToNot(HaveOccurred())

				results := make(chan int, 100)

				go func() {
					batch, _ := rx.Recv()
					results <- len(batch.GetBatch())
				}()

				Consistently(results, 3).Should(BeEmpty())
			})
		})
	})
})

func genericLogEnvelope() *loggregator_v2.Envelope {
	return &loggregator_v2.Envelope{
		Timestamp: time.Now().UnixNano(),
		Message: &loggregator_v2.Envelope_Log{
			Log: &loggregator_v2.Log{
				Payload: []byte("hello world"),
			},
		},
	}
}

func createRouterIngressClient(addr string, g app.GRPC) loggregator_v2.DopplerIngressClient {
	creds, err := plumbing.NewClientCredentials(
		g.CertFile,
		g.KeyFile,
		g.CAFile,
		"doppler",
	)
	if err != nil {
		panic(err)
	}

	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(creds))
	if err != nil {
		panic(err)
	}

	return loggregator_v2.NewDopplerIngressClient(conn)
}

func createRouterV2EgressClient(addr string, g app.GRPC) loggregator_v2.EgressClient {
	creds, err := plumbing.NewClientCredentials(
		g.CertFile,
		g.KeyFile,
		g.CAFile,
		"doppler",
	)
	if err != nil {
		panic(err)
	}

	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(creds))
	if err != nil {
		panic(err)
	}

	return loggregator_v2.NewEgressClient(conn)
}
