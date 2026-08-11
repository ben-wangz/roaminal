package connection

import (
	"context"
	"errors"
	"sync"

	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
	"github.com/ben-wangz/roaminal/backend/internal/persistence"
	"github.com/ben-wangz/roaminal/backend/internal/sshconfig"
	"github.com/ben-wangz/roaminal/backend/internal/sshkey"
	"github.com/ben-wangz/roaminal/backend/internal/terminal"
	"github.com/ben-wangz/roaminal/backend/internal/worker"
)

type Manager struct {
	*terminal.Manager
	configRepo  *sshconfig.Repository
	keys        *sshkey.Inventory
	options     *connectionoptions.Store
	sshPath     string
	runtimeDir  string
	transportMu sync.Mutex
	transports  map[string]*Transport
	instances   map[string]*Transport
	remoteMu    sync.Mutex
	remoteState map[string]*remoteMonitorState
	remoteSem   chan struct{}
	watchCancel context.CancelFunc
}

type Summary = terminal.Summary
type Client = terminal.Client
type ExitStatus = terminal.ExitStatus

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

func transportAcceptsReuse(transport *Transport) bool {
	return transport != nil && transport.Channels > 0 && !transport.Draining
}

var ErrClientCapacity = terminal.ErrClientCapacity
var ErrControlNotOwner = terminal.ErrControlNotOwner
var ErrTransportUnavailable = errors.New("ssh transport unavailable")
var ErrTransportDraining = errors.New("ssh transport is draining")
var ErrTmuxNotEnabled = errors.New("tmux is not enabled for this connection")

func NewManager(cfg config.Config, store *persistence.Store, terminalWorker *worker.Client) *Manager {
	return &Manager{Manager: terminal.NewManager(cfg, store, terminalWorker), sshPath: discover("ssh"), transports: make(map[string]*Transport), instances: make(map[string]*Transport), remoteState: make(map[string]*remoteMonitorState), remoteSem: make(chan struct{}, 4)}
}

func (m *Manager) SetRuntimeID(id string) {
	m.Manager.SetRuntimeID(id)
	dir, err := prepareRuntimeDir(id)
	if err != nil {
		// Local connections remain usable when the temporary mux root cannot
		// be prepared; remote creation reports an unavailable capability.
		m.runtimeDir = ""
		return
	}
	m.runtimeDir = dir
}

func (m *Manager) SetSources(repo *sshconfig.Repository, keys *sshkey.Inventory) {
	m.configRepo, m.keys = repo, keys
}

func (m *Manager) SetConnectionOptions(store *connectionoptions.Store) { m.options = store }

func (m *Manager) tmuxOptions(aliases map[string]bool) (map[string]connectionoptions.Tmux, error) {
	if m.options == nil {
		return map[string]connectionoptions.Tmux{}, nil
	}
	collection, err := m.options.Load(aliases)
	if err != nil {
		return nil, err
	}
	return collection.Options, nil
}

func (m *Manager) Start(ctx context.Context) error {
	if m.keys != nil {
		m.keys.CleanupStaging()
	}
	if err := m.Manager.Start(ctx); err != nil {
		return err
	}
	if m.configRepo != nil {
		watchCtx, cancel := context.WithCancel(context.Background())
		m.watchCancel = cancel
		go m.watchSources(watchCtx)
	}
	return nil
}

func (m *Manager) Shutdown(ctx context.Context) {
	if m.watchCancel != nil {
		m.watchCancel()
		m.watchCancel = nil
	}
	m.transportMu.Lock()
	transports := make([]*Transport, 0, len(m.transports))
	for _, transport := range m.transports {
		transports = append(transports, transport)
	}
	m.transports = make(map[string]*Transport)
	m.instances = make(map[string]*Transport)
	m.transportMu.Unlock()
	m.remoteMu.Lock()
	for _, state := range m.remoteState {
		if state != nil && state.inflight != nil {
			close(state.inflight)
		}
	}
	m.remoteState = make(map[string]*remoteMonitorState)
	m.remoteMu.Unlock()
	for _, transport := range transports {
		m.stopTransport(ctx, transport)
	}
	m.Manager.Shutdown(ctx)
}

func (m *Manager) Close(ctx context.Context, id string) error {
	err := m.Manager.Close(ctx, id)
	m.finishInstance(ctx, id, err == nil)
	return err
}
func (m *Manager) Delete(ctx context.Context, id string) error {
	err := m.Manager.Delete(ctx, id)
	m.finishInstance(ctx, id, err == nil)
	return err
}

func (m *Manager) finishInstance(ctx context.Context, id string, closed bool) {
	if !closed {
		return
	}
	m.transportMu.Lock()
	transport := m.instances[id]
	delete(m.instances, id)
	if transport != nil {
		if transport.Channels > 0 {
			transport.Channels--
		}
		if id == transport.OwnerID {
			transport.OwnerClosed = true
		}
		if transport.OwnerClosed && transport.Channels == 0 && transport.AuxiliaryChannels == 0 {
			delete(m.transports, transport.OwnerID)
		}
	}
	m.transportMu.Unlock()
	if transport != nil && transport.OwnerClosed && transport.Channels == 0 && transport.AuxiliaryChannels == 0 {
		m.clearRemoteState(transport.OwnerID)
		m.stopTransport(ctx, transport)
	}
}
