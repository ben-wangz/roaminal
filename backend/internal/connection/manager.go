package connection

import (
	"context"
	"sync"

	"github.com/ben-wangz/roaminal/backend/internal/clock"
	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/connectionoptions"
	"github.com/ben-wangz/roaminal/backend/internal/monitor"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"github.com/ben-wangz/roaminal/backend/internal/random"
	"github.com/ben-wangz/roaminal/backend/internal/sshconfig"
	"github.com/ben-wangz/roaminal/backend/internal/sshkey"
)

type Manager struct {
	instances     *InstanceService
	configRepo    *sshconfig.Repository
	keys          *sshkey.Inventory
	options       *connectionoptions.Store
	sshPath       string
	runtimeDir    string
	transportPool *TransportPool
	remoteMonitor *monitor.RemoteMonitorService
	watchCancel   context.CancelFunc
	clock         ports.Clock
	ids           ports.IDGenerator
	random        ports.RandomSource
}

type Summary = ports.ConnectionInstanceSummary
type ExitStatus = ports.TerminalExitStatus

type RemoteTransferInfo = ports.RemoteTransferInfo

func (m *Manager) ConnectionInstance(id string) (ports.ConnectionInstanceView, error) {
	for _, summary := range m.Summaries() {
		if summary.ID == id || summary.ConnectionInstanceID == id {
			return ports.ConnectionInstanceView{ID: summary.ID, ConnectionInstanceID: summary.ConnectionInstanceID, ConnectionDefinitionID: summary.ConnectionDefinitionID, Purpose: summary.Purpose, Type: summary.Type, Lifecycle: summary.Lifecycle, SourceState: summary.SourceState, SourceHostAlias: summary.SourceHostAlias, TmuxEnabled: summary.TmuxEnabled, TmuxSessionName: summary.TmuxSessionName}, nil
		}
	}
	return ports.ConnectionInstanceView{}, ports.ErrRemoteInstanceNotFound
}

func (m *Manager) ConnectionInstanceViews() []ports.ConnectionInstanceView {
	summaries := m.Summaries()
	views := make([]ports.ConnectionInstanceView, 0, len(summaries))
	for _, summary := range summaries {
		views = append(views, ports.ConnectionInstanceView{ID: summary.ID, ConnectionInstanceID: summary.ConnectionInstanceID, ConnectionDefinitionID: summary.ConnectionDefinitionID, Purpose: summary.Purpose, Type: summary.Type, Lifecycle: summary.Lifecycle, SourceState: summary.SourceState, SourceHostAlias: summary.SourceHostAlias, TmuxEnabled: summary.TmuxEnabled, TmuxSessionName: summary.TmuxSessionName})
	}
	return views
}

type Dependencies struct {
	Config     config.Config
	Runtime    ports.TerminalRuntime
	ConfigRepo *sshconfig.Repository
	Keys       *sshkey.Inventory
	Options    *connectionoptions.Store
	Clock      ports.Clock
	IDs        ports.IDGenerator
	Random     ports.RandomSource
}

func (m *Manager) RemoteTransferInfo(id string) (RemoteTransferInfo, error) {
	transport, err := m.remoteTransport(id)
	if err != nil {
		return RemoteTransferInfo{}, err
	}
	if m.sshPath == "" {
		return RemoteTransferInfo{}, ErrTransportUnavailable
	}
	return RemoteTransferInfo{Alias: transport.Alias, ControlPath: transport.ControlPath, SSHPath: m.sshPath}, nil
}

type remoteTransferLease struct {
	manager   *Manager
	transport *Transport
	info      RemoteTransferInfo
	once      sync.Once
}

func (l *remoteTransferLease) Info() RemoteTransferInfo { return l.info }

func (l *remoteTransferLease) Close() {
	l.once.Do(func() {
		l.manager.releaseAuxiliary(l.transport)
	})
}

func (m *Manager) AcquireRemoteTransfer(ctx context.Context, id string) (ports.RemoteTransferLease, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	transport, err := m.auxiliaryTransport(ctx, id)
	if err != nil {
		return nil, err
	}
	return &remoteTransferLease{
		manager:   m,
		transport: transport,
		info:      RemoteTransferInfo{Alias: transport.Alias, ControlPath: transport.ControlPath, SSHPath: m.sshPath},
	}, nil
}

var ErrRemoteInstanceNotFound = ports.ErrRemoteInstanceNotFound
var ErrRemoteNoTransport = ports.ErrRemoteNoTransport
var ErrClientCapacity = ports.ErrClientCapacity
var ErrConnectionCapacity = ports.ErrConnectionCapacity
var ErrControlNotOwner = ports.ErrControlNotOwner
var ErrTransportUnavailable = ports.ErrTransportUnavailable
var ErrTransportDraining = ports.ErrTransportDraining
var ErrTmuxNotEnabled = ports.ErrTmuxNotEnabled

func NewManager(deps Dependencies) *Manager {
	runtimeClock := deps.Clock
	if runtimeClock == nil {
		runtimeClock = clock.System{}
	}
	randomSource := deps.Random
	if randomSource == nil {
		randomSource = random.CryptoSource{}
	}
	idGenerator := deps.IDs
	manager := &Manager{instances: NewInstanceService(deps.Runtime), configRepo: deps.ConfigRepo, keys: deps.Keys, options: deps.Options, sshPath: discover("ssh"), transportPool: newTransportPool(), clock: runtimeClock, ids: idGenerator, random: randomSource}
	manager.remoteMonitor = monitor.NewRemoteMonitorService(manager, monitor.Dependencies{Clock: runtimeClock, Random: randomSource})
	runtimeID := ""
	if deps.Runtime != nil {
		runtimeID = deps.Runtime.RuntimeID()
	}
	dir, err := manager.prepareRuntimeDir(runtimeID)
	if err != nil {
		// Local connections remain usable when the temporary mux root cannot be
		// prepared; remote creation reports an unavailable capability.
		return manager
	}
	manager.runtimeDir = dir
	return manager
}

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
	if err := m.instances.Start(ctx); err != nil {
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
	m.transportPool.mu.Lock()
	transports := make([]*Transport, 0, len(m.transportPool.transports))
	for _, transport := range m.transportPool.transports {
		transports = append(transports, transport)
	}
	m.transportPool.transports = make(map[string]*Transport)
	m.transportPool.instances = make(map[string]*Transport)
	m.transportPool.mu.Unlock()
	if m.remoteMonitor != nil {
		m.remoteMonitor.Close()
	}
	for _, transport := range transports {
		m.stopTransport(ctx, transport)
	}
	m.instances.Shutdown(ctx)
}

func (m *Manager) Delete(ctx context.Context, id string) error {
	err := m.instances.Delete(ctx, id)
	m.finishInstance(ctx, id, err == nil)
	return err
}

func (m *Manager) finishInstance(ctx context.Context, id string, closed bool) {
	if !closed {
		return
	}
	m.transportPool.mu.Lock()
	transport := m.transportPool.instances[id]
	delete(m.transportPool.instances, id)
	shouldStop := false
	if transport != nil {
		if transport.Channels > 0 {
			transport.Channels--
		}
		if id == transport.OwnerID {
			transport.OwnerClosed = true
		}
		shouldStop = transport.OwnerClosed && transport.Channels == 0 && transport.AuxiliaryChannels == 0
		if shouldStop {
			delete(m.transportPool.transports, transport.OwnerID)
		}
	}
	m.transportPool.mu.Unlock()
	if transport != nil && shouldStop {
		m.clearRemoteState(transport.OwnerID)
		m.stopTransport(ctx, transport)
	}
}
