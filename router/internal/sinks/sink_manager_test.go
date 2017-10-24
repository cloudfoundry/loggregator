package sinks_test

import (
	"sync"
	"time"

	"code.cloudfoundry.org/loggregator/metricemitter/testhelper"
	"code.cloudfoundry.org/loggregator/router/internal/sinks"
	"github.com/cloudfoundry/dropsonde/emitter"
	"github.com/cloudfoundry/dropsonde/factories"
	"github.com/cloudfoundry/sonde-go/events"
	"github.com/gogo/protobuf/proto"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("SinkManager", func() {
	var (
		sinkManager *sinks.SinkManager
	)

	BeforeEach(func() {
		fakeMetricSender.Reset()

		health := newSpyHealthRegistrar()
		sinkManager = sinks.NewSinkManager(
			1,
			100,
			"dropsonde-origin",
			1*time.Second,
			1*time.Second,
			nil,
			testhelper.NewMetricClient(),
			health,
		)
	})

	AfterEach(func() {
		sinkManager.Stop()
	})

	Describe("SendTo", func() {
		It("sends to all known sinks", func() {
			sink1 := &channelSink{appId: "myApp",
				identifier: "myAppChan1",
				done:       make(chan struct{}),
			}
			sink2 := &channelSink{appId: "myApp",
				identifier: "myAppChan2",
				done:       make(chan struct{}),
			}

			sinkManager.RegisterSink(sink1)
			sinkManager.RegisterSink(sink2)

			expectedMessageString := "Some Data"
			expectedMessage, _ := emitter.Wrap(
				factories.NewLogMessage(
					events.LogMessage_OUT,
					expectedMessageString,
					"myApp",
					"App",
				),
				"origin",
			)
			go sinkManager.SendTo("myApp", expectedMessage)

			Eventually(sink1.Received).Should(HaveLen(1))
			Eventually(sink2.Received).Should(HaveLen(1))
			Expect(sink1.Received()[0]).To(Equal(expectedMessage))
			Expect(sink2.Received()[0]).To(Equal(expectedMessage))
		})

		It("only sends to sinks that match the appID", func(done Done) {
			sink1 := &channelSink{appId: "myApp1",
				identifier: "myAppChan1",
				done:       make(chan struct{}),
			}
			sink2 := &channelSink{appId: "myApp2",
				identifier: "myAppChan2",
				done:       make(chan struct{}),
			}

			sinkManager.RegisterSink(sink1)
			sinkManager.RegisterSink(sink2)

			expectedMessageString := "Some Data"
			expectedMessage, _ := emitter.Wrap(
				factories.NewLogMessage(
					events.LogMessage_OUT,
					expectedMessageString,
					"myApp",
					"App",
				),
				"origin",
			)
			go sinkManager.SendTo("myApp1", expectedMessage)

			Eventually(sink1.Received).Should(HaveLen(1))
			Expect(sink1.Received()[0]).To(Equal(expectedMessage))
			Expect(sink2.Received()).To(BeEmpty())

			close(done)
		})

		It("sends a message to app sink registered after a message was sent", func() {
			sink1 := &channelSink{appId: "myApp",
				identifier: "myAppChan1",
				done:       make(chan struct{}),
			}

			expectedMessageString := "Some Data"
			expectedMessage, _ := emitter.Wrap(
				factories.NewLogMessage(
					events.LogMessage_OUT,
					expectedMessageString,
					"myApp",
					"App",
				),
				"origin",
			)
			go sinkManager.SendTo("myApp", expectedMessage)

			sinkManager.RegisterSink(sink1)

			Eventually(sink1.Received).Should(HaveLen(1))
			Expect(sink1.Received()[0]).To(Equal(expectedMessage))
		})

		It("buffers a messages when a sink is consuming slowly", func() {
			ready := make(chan struct{})
			sink1 := &channelSink{appId: "myApp",
				identifier: "myAppChan1",
				done:       make(chan struct{}),
				ready:      ready,
			}

			sinkManager.RegisterSink(sink1)

			expectedFirstMessageString := "Some Data 1"
			expectedSecondMessageString := "Some Data 2"
			expectedFirstMessage, _ := emitter.Wrap(
				factories.NewLogMessage(
					events.LogMessage_OUT,
					expectedFirstMessageString,
					"myApp",
					"App",
				),
				"origin",
			)
			expectedSecondMessage, _ := emitter.Wrap(
				factories.NewLogMessage(
					events.LogMessage_OUT,
					expectedSecondMessageString,
					"myApp",
					"App",
				),
				"origin",
			)

			sinkManager.SendTo("myApp", expectedFirstMessage)
			sinkManager.SendTo("myApp", expectedSecondMessage)

			close(ready)

			Eventually(sink1.Received).Should(HaveLen(2))
			Expect(sink1.Received()[0]).To(Equal(expectedFirstMessage))
			Expect(sink1.Received()[1]).To(Equal(expectedSecondMessage))
		})
	})

	Describe("Stop", func() {
		It("stops all registered sinks", func(done Done) {
			sink := &channelSink{appId: "myApp1",
				identifier: "myAppChan1",
				done:       make(chan struct{}),
			}
			sinkManager.RegisterSink(sink)
			sinkManager.Stop()
			Expect(sink.RunFinished()).To(BeTrue())

			close(done)
		})

		It("is idempotent", func() {
			sinkManager.Stop()
			Expect(sinkManager.Stop).NotTo(Panic())
		})
	})

	Describe("UnregisterSink", func() {
		Context("with a DumpSink", func() {
			var dumpSink *sinks.DumpSink

			BeforeEach(func() {
				health := newSpyHealthRegistrar()
				dumpSink = sinks.NewDumpSink("appId", 1, time.Hour, health)
				sinkManager.RegisterSink(dumpSink)
			})

			It("clears the recent logs buffer", func() {
				expectedMessageString := "Some Data"
				expectedMessage, _ := emitter.Wrap(
					factories.NewLogMessage(
						events.LogMessage_OUT,
						expectedMessageString,
						"myApp",
						"App",
					),
					"origin",
				)
				sinkManager.SendTo("appId", expectedMessage)

				Eventually(func() []*events.Envelope {
					return sinkManager.RecentLogsFor("appId")
				}).Should(HaveLen(1))

				sinkManager.UnregisterSink(dumpSink)

				Eventually(func() []*events.Envelope {
					return sinkManager.RecentLogsFor("appId")
				}).Should(HaveLen(0))
			})
		})

		Context("when called twice", func() {
			var dumpSink *sinks.DumpSink

			BeforeEach(func() {
				health := newSpyHealthRegistrar()
				dumpSink = sinks.NewDumpSink("appId", 1, time.Hour, health)
				sinkManager.RegisterSink(dumpSink)
			})

			It("decrements the metric only once", func() {
				f := func() float64 {
					return fakeMetricSender.GetValue(
						"messageRouter.numberOfDumpSinks",
					).Value
				}
				Eventually(f, 2).Should(BeEquivalentTo(1))

				sinkManager.UnregisterSink(dumpSink)

				Eventually(f, 2).Should(BeEquivalentTo(0))

				sinkManager.UnregisterSink(dumpSink)

				Eventually(f, 2).Should(BeEquivalentTo(0))
			})
		})
	})

	Describe("Latest Container Metrics", func() {
		var sink *channelSink
		BeforeEach(func() {
			sink = &channelSink{
				appId:      "myApp",
				identifier: "myAppChan1",
				done:       make(chan struct{}),
			}
			sinkManager.RegisterSink(sink)
		})

		It("sends latest container metrics for a given app", func() {
			env := &events.Envelope{
				EventType: events.Envelope_ContainerMetric.Enum(),
				Timestamp: proto.Int64(time.Now().UnixNano()),
				ContainerMetric: &events.ContainerMetric{
					ApplicationId: proto.String("myApp"),
					InstanceIndex: proto.Int32(1),
					CpuPercentage: proto.Float64(73),
					MemoryBytes:   proto.Uint64(2),
					DiskBytes:     proto.Uint64(3),
				},
			}

			sinkManager.SendTo("myApp", env)

			f := func() []*events.Envelope {
				return sinkManager.LatestContainerMetrics("myApp")
			}
			Eventually(f).Should(ConsistOf(env))
		})
	})

})

type channelSink struct {
	sync.RWMutex
	done              chan struct{}
	appId, identifier string
	received          []*events.Envelope
	runCalled         bool
	ready             chan struct{}
	stop              chan struct{}
}

func (c *channelSink) AppID() string { return c.appId }
func (c *channelSink) Run(msgChan <-chan *events.Envelope) {
	if c.ready != nil {
		<-c.ready
	}
	c.Lock()
	c.runCalled = true
	c.Unlock()

	defer close(c.done)
	for {
		select {
		case msg, ok := <-msgChan:
			if !ok {
				return
			}
			c.Lock()
			c.received = append(c.received, msg)
			c.Unlock()
		case <-c.stop:
			return
		}
	}
}

func (c *channelSink) RunFinished() bool {
	<-c.done
	return true
}

func (c *channelSink) RunCalled() bool {
	c.RLock()
	defer c.RUnlock()
	return c.runCalled
}

func (c *channelSink) Received() []*events.Envelope {
	c.RLock()
	defer c.RUnlock()
	data := make([]*events.Envelope, len(c.received))
	copy(data, c.received)
	return data
}

func (c *channelSink) Identifier() string { return c.identifier }

func (c *channelSink) GetInstrumentationMetric() sinks.Metric {
	return sinks.Metric{Name: "numberOfMessagesLost", Value: 25}
}
