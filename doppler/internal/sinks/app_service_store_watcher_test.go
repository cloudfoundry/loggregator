package sinks_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sync"

	"code.cloudfoundry.org/loggregator/doppler/internal/sinks"
	"github.com/cloudfoundry/storeadapter"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("AppServiceStoreWatcher", func() {
	var (
		one sinks.ServiceInfo
		two sinks.ServiceInfo
	)

	BeforeEach(func() {
		one = sinks.NewServiceInfo(
			"app-1",
			"syslog://example.com:12345",
			"org.space.app-one.1",
		)
		two = sinks.NewServiceInfo(
			"app-1",
			"syslog://example.com:12346",
			"org.space.app-one.1",
		)
	})

	It("watches the Loggregator v2 namespace", func() {
		adapter := newSpyStore()
		watcher, _, _ := sinks.NewAppServiceStoreWatcher(
			adapter,
			sinks.NewAppServiceCache(),
		)

		go watcher.Run()

		Eventually(adapter.watchDir).Should(Equal("/loggregator/v2/services"))
	})

	It("closes the outgoing channels on shutdown", func() {
		adapter := newSpyStore()
		watcher, addCh, removeCh := sinks.NewAppServiceStoreWatcher(
			adapter,
			sinks.NewAppServiceCache(),
		)

		go watcher.Run()
		watcher.Stop()

		Eventually(addCh).Should(BeClosed())
		Eventually(removeCh).Should(BeClosed())
	})

	It("calls watch again when there is an error", func() {
		adapter := newSpyStore()
		watcher, _, _ := sinks.NewAppServiceStoreWatcher(
			adapter,
			sinks.NewAppServiceCache(),
		)

		go watcher.Run()

		Eventually(adapter.watchCalled).Should(Equal(1))
		adapter.setWatchError(errors.New("Haha"))
		Eventually(adapter.watchCalled).Should(Equal(2))
	})

	It("does not send on the output channels when the store is empty", func() {
		adapter := newSpyStore()
		watcher, addCh, removeCh := sinks.NewAppServiceStoreWatcher(
			adapter,
			sinks.NewAppServiceCache(),
		)

		go watcher.Run()

		Consistently(addCh).Should(BeEmpty())
		Consistently(removeCh).Should(BeEmpty())
	})

	It("sends app services on the output add channel", func() {
		adapter := newSpyStore()

		a := buildNode(one)
		adapter.addNode(a)
		b := buildNode(two)
		adapter.addNode(b)

		watcher, addCh, removeCh := sinks.NewAppServiceStoreWatcher(
			adapter,
			sinks.NewAppServiceCache(),
		)

		go watcher.Run()

		appServices := drainOutgoingChannel(addCh, 2)
		Expect(appServices).To(ContainElement(one))
		Expect(appServices).To(ContainElement(two))
		Expect(removeCh).To(BeEmpty())
	})

	It("ignores invalid JSON", func() {
		adapter := newSpyStore()
		node := storeadapter.StoreNode{
			Key:   "/loggregator/v2/services/some-app-id/some-url-id",
			Value: []byte(`{"invalid: "json"}`),
		}
		adapter.addNode(node)
		watcher, addCh, _ := sinks.NewAppServiceStoreWatcher(
			adapter,
			sinks.NewAppServiceCache(),
		)

		go watcher.Run()

		Consistently(addCh).ShouldNot(Receive())
	})

	It("does not send updates when the data has already been processed", func() {
		adapter := newSpyStore()
		adapter.addNode(buildNode(one))
		watcher, addCh, removeCh := sinks.NewAppServiceStoreWatcher(
			adapter,
			sinks.NewAppServiceCache(),
		)

		go watcher.Run()

		drainOutgoingChannel(addCh, 1)

		adapter.addNode(buildNode(one))

		Consistently(addCh).Should(BeEmpty())
		Consistently(removeCh).Should(BeEmpty())
	})

	It("sends a services to be removed on the output remove channel", func() {
		adapter := newSpyStore()
		adapter.addNode(buildNode(one)) // add it to the cache
		adapter.removeNode(buildNode(one))
		watcher, addCh, removeCh := sinks.NewAppServiceStoreWatcher(
			adapter,
			sinks.NewAppServiceCache(),
		)

		go watcher.Run()

		drainOutgoingChannel(addCh, 1) // drain add events

		var appService sinks.AppService
		Eventually(removeCh).Should(Receive(&appService))
		Expect(appService).To(Equal(one))
		Consistently(addCh).Should(BeEmpty())
	})

	It("removes all services when an application goes away", func() {
		adapter := newSpyStore()
		adapter.addNode(buildNode(one)) // add it to the cache
		adapter.removeNode(buildDirNode("app-1"))
		watcher, addCh, removeCh := sinks.NewAppServiceStoreWatcher(
			adapter,
			sinks.NewAppServiceCache(),
		)

		go watcher.Run()

		drainOutgoingChannel(addCh, 1) // drain add events

		var appService sinks.AppService
		Eventually(removeCh).Should(Receive(&appService))
		Expect(appService).To(Equal(one))
		Consistently(addCh).Should(BeEmpty())
	})

	It("removes the app service from the cache when a service expires", func() {
		adapter := newSpyStore()
		adapter.addNode(buildNode(one)) // add it to the cache
		adapter.expireNode(buildNode(one))
		watcher, addCh, removeCh := sinks.NewAppServiceStoreWatcher(
			adapter,
			sinks.NewAppServiceCache(),
		)

		go watcher.Run()

		drainOutgoingChannel(addCh, 1) // drain add events

		var appService sinks.AppService
		Eventually(removeCh).Should(Receive(&appService))
		Expect(appService).To(Equal(one))
		Consistently(addCh).Should(BeEmpty())
	})
})

func key(service sinks.ServiceInfo) string {
	return path.Join("/loggregator/v2/services", service.AppId(), service.Id())
}

func drainOutgoingChannel(c <-chan sinks.AppService, count int) []sinks.AppService {
	var appServices []sinks.AppService

	for i := 0; i < count; i++ {
		var appService sinks.AppService
		Eventually(c).Should(Receive(&appService),
			fmt.Sprintf("Failed to drain outgoing chan with expected number of messages; received %d but expected %d.", i, count),
		)
		appServices = append(appServices, appService)
	}

	return appServices
}

func buildDirNode(id string) storeadapter.StoreNode {
	return storeadapter.StoreNode{
		Key: path.Join("/loggregator/v2/services", id),
		Dir: true,
	}
}

func buildNode(appService sinks.AppService) storeadapter.StoreNode {
	m := metaData{Hostname: appService.Hostname(), DrainURL: appService.Url()}

	data, err := json.Marshal(&m)
	Expect(err).ToNot(HaveOccurred())

	return storeadapter.StoreNode{
		Key:   path.Join("/loggregator/v2/services", appService.AppId(), appService.Id()),
		Value: data,
	}
}

type metaData struct {
	Hostname string `json:"hostname"`
	DrainURL string `json:"drainURL"`
}

type spyStore struct {
	mu           sync.Mutex
	watchCalled_ int
	watchEvent   chan storeadapter.WatchEvent
	watchErr     chan error
	stop_        chan bool
	watchDir_    string
}

func newSpyStore() *spyStore {
	return &spyStore{
		watchEvent: make(chan storeadapter.WatchEvent, 10),
		watchErr:   make(chan error, 10),
		stop_:      make(chan bool),
	}
}

func (s *spyStore) watchCalled() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.watchCalled_
}

func (s *spyStore) setWatchError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.watchErr <- err
}

func (s *spyStore) addNode(n storeadapter.StoreNode) {
	s.watchEvent <- storeadapter.WatchEvent{
		Type: storeadapter.CreateEvent,
		Node: &n,
	}
}

func (s *spyStore) removeNode(n storeadapter.StoreNode) {
	s.watchEvent <- storeadapter.WatchEvent{
		Type:     storeadapter.DeleteEvent,
		PrevNode: &n,
	}
}

func (s *spyStore) expireNode(n storeadapter.StoreNode) {
	s.watchEvent <- storeadapter.WatchEvent{
		Type:     storeadapter.ExpireEvent,
		PrevNode: &n,
	}
}

func (s *spyStore) watchDir() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.watchDir_
}

func (s *spyStore) Watch(key string) (<-chan storeadapter.WatchEvent, chan<- bool, <-chan error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.watchCalled_++
	s.watchDir_ = key
	return s.watchEvent, s.stop_, s.watchErr
}

func (s *spyStore) ListRecursively(key string) (storeadapter.StoreNode, error) {
	return storeadapter.StoreNode{}, nil
}
