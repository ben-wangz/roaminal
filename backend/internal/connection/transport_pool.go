package connection

import "sync"

// TransportPool owns SSH control-channel reuse and the association between a
// user-visible connection instance and its transport owner. Connection
// lifecycle code may ask the pool for handles, but it cannot own or expose the
// maps directly outside this package.
type TransportPool struct {
	mu         sync.Mutex
	transports map[string]*Transport
	instances  map[string]*Transport
}

type Transport struct {
	Alias              string
	ControlPath        string
	SourceRevision     string
	TmuxLaunchRevision string
	OwnerID            string
	Channels           int
	AuxiliaryChannels  int
	OwnerClosed        bool
	Draining           bool
	stopRequested      bool
}

func newTransportPool() *TransportPool {
	return &TransportPool{transports: make(map[string]*Transport), instances: make(map[string]*Transport)}
}

func transportAcceptsReuse(transport *Transport) bool {
	return transport != nil && transport.Channels > 0 && !transport.Draining
}
