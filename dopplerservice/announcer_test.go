package dopplerservice_test

import (
	"errors"
	"fmt"
	"time"

	"code.cloudfoundry.org/loggregator/dopplerservice"

	"github.com/cloudfoundry/storeadapter"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Announcer", func() {
	var (
		conf = dopplerservice.Config{
			JobName:      "doppler_z1",
			Index:        "0",
			Zone:         "z1",
			OutgoingPort: 8888,
		}
		// legacyKey  = "/healthstatus/doppler/z1/doppler_z1/0"
		// dopplerKey = "/doppler/meta/z1/doppler_z1/0"
	)

	It("creates a node", func() {
		store := &spyDopplerConfigStore{}

		dopplerservice.Announce("127.0.0.1", time.Second, &conf, store)

		expected := storeadapter.StoreNode{
			Key:   "/doppler/meta/z1/doppler_z1/0",
			Value: []byte("{\"version\":1,\"endpoints\":[\"ws://127.0.0.1:8888\"]}"),
			TTL:   uint64(time.Second.Seconds()),
		}
		Expect(store.createNode).To(Equal(expected))
	})

	It("maintains a node", func() {
		store := &spyDopplerConfigStore{}

		dopplerservice.Announce("127.0.0.1", time.Second, &conf, store)

		expected := storeadapter.StoreNode{
			Key:   "/doppler/meta/z1/doppler_z1/0",
			Value: []byte("{\"version\":1,\"endpoints\":[\"ws://127.0.0.1:8888\"]}"),
			TTL:   uint64(time.Second.Seconds()),
		}
		Expect(store.maintainNode).To(Equal(expected))
	})

	It("panics when maintaining a node fails", func() {
		store := &spyDopplerConfigStore{}
		store.maintainNodeErr = errors.New("maintain failed")

		Expect(func() {
			dopplerservice.Announce("127.0.0.1", time.Second, &conf, store)
		}).To(Panic())
	})

	It("announces only udp and websocket", func() {
		store := &spyDopplerConfigStore{}

		dopplerservice.Announce("127.0.0.1", time.Second, &conf, store)

		expected := fmt.Sprintf(
			`{"version": 1, "endpoints":["ws://%[1]s:8888" ]}`,
			"127.0.0.1",
		)
		Expect(store.createNode.Value).To(MatchJSON(expected))
	})

	It("maintains the legacy node", func() {
		store := &spyDopplerConfigStore{}

		dopplerservice.AnnounceLegacy("127.0.0.1", time.Second, &conf, store)

		expected := storeadapter.StoreNode{
			Key:   "/healthstatus/doppler/z1/doppler_z1/0",
			Value: []byte("127.0.0.1"),
			TTL:   uint64(time.Second.Seconds()),
		}
		Expect(store.maintainNode).To(Equal(expected))
	})

	It("panics when MaintainNode returns error for the legacy node", func() {
		store := &spyDopplerConfigStore{}
		store.maintainNodeErr = errors.New("maintain failed")

		Expect(func() {
			dopplerservice.AnnounceLegacy("127.0.0.1", time.Second, &conf, store)
		}).To(Panic())
	})
})

type spyDopplerConfigStore struct {
	createNode      storeadapter.StoreNode
	maintainNode    storeadapter.StoreNode
	maintainNodeErr error
}

func (s *spyDopplerConfigStore) Create(n storeadapter.StoreNode) error {
	s.createNode = n

	return nil
}

func (s *spyDopplerConfigStore) MaintainNode(n storeadapter.StoreNode) (<-chan bool, chan chan bool, error) {
	s.maintainNode = n
	return make(chan bool), make(chan chan bool), s.maintainNodeErr
}
