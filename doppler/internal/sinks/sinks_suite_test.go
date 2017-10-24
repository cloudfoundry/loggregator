package sinks_test

import (
	"io/ioutil"
	"log"
	"testing"

	"github.com/cloudfoundry/dropsonde/emitter/fake"
	fakeMS "github.com/cloudfoundry/dropsonde/metric_sender/fake"
	"github.com/cloudfoundry/dropsonde/metrics"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc/grpclog"
)

var (
	fakeMetricSender *fakeMS.FakeMetricSender
	fakeEventEmitter *fake.FakeEventEmitter
)

func TestSinks(t *testing.T) {
	log.SetOutput(GinkgoWriter)
	grpclog.SetLogger(log.New(ioutil.Discard, "", 0))
	RegisterFailHandler(Fail)
	RunSpecs(t, "Sinks Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	return nil
}, func(encodedBuiltArtifacts []byte) {
	fakeEventEmitter = fake.NewFakeEventEmitter("doppler")
	fakeMetricSender = fakeMS.NewFakeMetricSender()

	metrics.Initialize(fakeMetricSender, nil)
}, 10)
