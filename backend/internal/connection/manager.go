package connection

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/config"
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
	sshPath     string
	runtimeDir  string
	transportMu sync.Mutex
	transports  map[string]*Transport
	instances   map[string]*Transport
	watchCancel context.CancelFunc
}

type Summary = terminal.Summary
type Client = terminal.Client
type ExitStatus = terminal.ExitStatus

type Transport struct {
	Alias           string
	ControlPath     string
	ContextRevision string
	OwnerID         string
	Channels        int
	OwnerClosed     bool
	Draining        bool
	stopRequested   bool
}

var ErrClientCapacity = terminal.ErrClientCapacity
var ErrControlNotOwner = terminal.ErrControlNotOwner
var ErrTransportUnavailable = errors.New("ssh transport unavailable")
var ErrTransportDraining = errors.New("ssh transport is draining")

func NewManager(cfg config.Config, store *persistence.Store, terminalWorker *worker.Client) *Manager {
	return &Manager{Manager: terminal.NewManager(cfg, store, terminalWorker), sshPath: discover("ssh"), transports: make(map[string]*Transport), instances: make(map[string]*Transport)}
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
	for _, transport := range transports {
		m.stopTransport(ctx, transport)
	}
	m.Manager.Shutdown(ctx)
}

func (m *Manager) CreateRemote(ctx context.Context, definitionID string, cols, rows int, reuseFrom string) (Summary, error) {
	if reuseFrom != "" {
		return m.createReuse(ctx, definitionID, cols, rows, reuseFrom)
	}
	if m.sshPath == "" || m.configRepo == nil || m.runtimeDir == "" {
		return Summary{}, ErrTransportUnavailable
	}
	alias, err := aliasFromDefinitionID(definitionID)
	if err != nil {
		return Summary{}, err
	}
	collection, err := m.configRepo.Collection(keySet(m.keys))
	if err != nil {
		return Summary{}, err
	}
	var definition *sshconfig.Definition
	for index := range collection.Definitions {
		if collection.Definitions[index].HostAlias == alias {
			definition = &collection.Definitions[index]
			break
		}
	}
	if definition == nil {
		return Summary{}, errors.New("connection definition not found")
	}
	id := terminalID()
	transportDir := filepath.Join(m.runtimeDir, "t-"+strings.TrimSuffix(id, "-"))
	if err := os.MkdirAll(transportDir, 0o700); err != nil {
		return Summary{}, err
	}
	controlPath := filepath.Join(transportDir, "ctl")
	transport := &Transport{Alias: alias, ControlPath: controlPath, ContextRevision: collection.ETag, OwnerID: id, Channels: 1}
	aliasPtr := alias
	meta := persistence.SessionMeta{ID: id, BackendRuntimeID: m.RuntimeID(), ConnectionDefinitionID: definitionID, Type: "ssh", Purpose: "interactive", SourceHostAlias: &aliasPtr, Lifecycle: "live", SourceState: "current", HostVerificationAssessment: definition.HostVerificationAssessment, Cols: cols, Rows: rows, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), AutomaticTitle: alias}
	argv := []string{m.sshPath, "-o", "ControlMaster=yes", "-o", "ControlPersist=yes", "-o", "ControlPath=" + controlPath, "--", alias}
	m.transportMu.Lock()
	m.transports[id] = transport
	m.instances[id] = transport
	m.transportMu.Unlock()
	result, err := m.CreateProcessWithExit(ctx, meta, argv, nil, func(_ terminal.ExitStatus) {
		m.finishInstance(context.Background(), id, true)
	})
	if err != nil {
		m.transportMu.Lock()
		delete(m.instances, id)
		delete(m.transports, id)
		m.transportMu.Unlock()
		_ = os.RemoveAll(transportDir)
		return Summary{}, err
	}
	if !m.transportReady(transport) {
		transport.Draining = false
	}
	return result, nil
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
		if transport.OwnerClosed && transport.Channels == 0 {
			delete(m.transports, transport.OwnerID)
		}
	}
	m.transportMu.Unlock()
	if transport != nil && transport.OwnerClosed && transport.Channels == 0 {
		m.stopTransport(ctx, transport)
	}
}
