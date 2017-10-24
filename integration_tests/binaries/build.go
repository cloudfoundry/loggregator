package binaries

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/onsi/gomega/gexec"
)

type BuildPaths struct {
	Metron            string `json:"metron"`
	Router            string `json:"router"`
	TrafficController string `json:"traffic_controller"`
}

func (bp BuildPaths) Marshal() ([]byte, error) {
	return json.Marshal(bp)
}

func (bp *BuildPaths) Unmarshal(text []byte) error {
	return json.Unmarshal(text, bp)
}

func (bp BuildPaths) SetEnv() {
	os.Setenv("METRON_BUILD_PATH", bp.Metron)
	os.Setenv("ROUTER_BUILD_PATH", bp.Router)
	os.Setenv("TRAFFIC_CONTROLLER_BUILD_PATH", bp.TrafficController)
}

func Build() (BuildPaths, chan error) {
	var bp BuildPaths
	errors := make(chan error, 100)
	defer close(errors)

	if os.Getenv("SKIP_BUILD") != "" {
		fmt.Println("Skipping building of binaries")
		bp.Metron = os.Getenv("METRON_BUILD_PATH")
		bp.Router = os.Getenv("ROUTER_BUILD_PATH")
		bp.TrafficController = os.Getenv("TRAFFIC_CONTROLLER_BUILD_PATH")
		return bp, errors
	}

	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)
	wg.Add(3)

	go func() {
		defer wg.Done()
		metronPath, err := gexec.Build("code.cloudfoundry.org/loggregator/metron", "-race")
		if err != nil {
			errors <- err
			return
		}
		mu.Lock()
		defer mu.Unlock()
		bp.Metron = metronPath
	}()

	go func() {
		defer wg.Done()
		routerPath, err := gexec.Build("code.cloudfoundry.org/loggregator/router", "-race")
		if err != nil {
			errors <- err
			return
		}
		mu.Lock()
		defer mu.Unlock()
		bp.Router = routerPath
	}()

	go func() {
		defer wg.Done()
		tcPath, err := gexec.Build("code.cloudfoundry.org/loggregator/trafficcontroller", "-race")
		if err != nil {
			errors <- err
			return
		}
		mu.Lock()
		defer mu.Unlock()
		bp.TrafficController = tcPath
	}()

	wg.Wait()
	return bp, errors
}

func Cleanup() {
	gexec.CleanupBuildArtifacts()
}
