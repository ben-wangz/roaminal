package agent

import (
	"sync"
	"testing"
)

func newStoreTestService(t *testing.T) *Service {
	t.Helper()
	return &Service{
		store:        OpenStore(t.TempDir()),
		bindings:     map[string]Target{},
		operations:   map[string]*Initialization{},
		endpointOps:  map[string]string{},
		endpointLock: map[string]*sync.Mutex{},
	}
}
