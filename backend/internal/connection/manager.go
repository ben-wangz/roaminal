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
	result, err := m.CreateProcess(ctx, meta, argv, nil)
	if err != nil {
		_ = os.RemoveAll(transportDir)
		return Summary{}, err
	}
	transport.OwnerID = result.ID
	m.transportMu.Lock()
	m.transports[result.ID] = transport
	m.instances[result.ID] = transport
	m.transportMu.Unlock()
	if !m.transportReady(transport) {
		transport.Draining = false
	}
	return result, nil
}

func (m *Manager) createReuse(ctx context.Context, definitionID string, cols, rows int, sourceID string) (Summary, error) {
	if m.sshPath == "" || m.configRepo == nil {
		return Summary{}, ErrTransportUnavailable
	}
	m.transportMu.Lock()
	transport := m.instances[sourceID]
	if transport != nil && transport.OwnerID != sourceID {
		transport = m.transports[transport.OwnerID]
	}
	m.transportMu.Unlock()
	if transport == nil {
		return Summary{}, ErrTransportUnavailable
	}
	var source Summary
	for _, item := range m.Manager.Summaries() {
		if item.ID == sourceID {
			source = item
			break
		}
	}
	if source.ID == "" || source.Type != "ssh" || source.Lifecycle != "live" || source.SourceState != "current" {
		return Summary{}, ErrTransportUnavailable
	}
	collection, err := m.configRepo.Collection(keySet(m.keys))
	if err != nil {
		return Summary{}, err
	}
	m.transportMu.Lock()
	draining, revision, alias := transport.Draining, transport.ContextRevision, transport.Alias
	m.transportMu.Unlock()
	if draining || revision != collection.ETag {
		m.transportMu.Lock()
		transport.Draining = true
		m.transportMu.Unlock()
		return Summary{}, ErrTransportDraining
	}
	if !m.transportReady(transport) {
		return Summary{}, ErrTransportUnavailable
	}
	definitionAlias, err := aliasFromDefinitionID(definitionID)
	if err != nil || definitionAlias != alias || source.SourceHostAlias == nil || *source.SourceHostAlias != definitionAlias {
		return Summary{}, errors.New("reuse definition does not match transport")
	}
	id := terminalID()
	aliasPtr := definitionAlias
	meta := persistence.SessionMeta{ID: id, BackendRuntimeID: m.RuntimeID(), ConnectionDefinitionID: definitionID, Type: "ssh", Purpose: "interactive", SourceHostAlias: &aliasPtr, Lifecycle: "live", SourceState: "current", Cols: cols, Rows: rows, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), AutomaticTitle: definitionAlias, ReuseFromConnectionInstanceID: &sourceID}
	argv := []string{m.sshPath, "-o", "ControlMaster=no", "-o", "ControlPersist=no", "-o", "ControlPath=" + transport.ControlPath, "-o", "CanonicalizeHostname=no", "-o", "ProxyCommand=/bin/false", "--", definitionAlias}
	m.transportMu.Lock()
	if transport.Draining || transport.OwnerClosed {
		m.transportMu.Unlock()
		return Summary{}, ErrTransportDraining
	}
	transport.Channels++
	m.transportMu.Unlock()
	result, err := m.CreateProcess(ctx, meta, argv, nil)
	if err != nil {
		m.transportMu.Lock()
		transport.Channels--
		m.transportMu.Unlock()
		return Summary{}, err
	}
	m.transportMu.Lock()
	m.instances[result.ID] = transport
	m.transportMu.Unlock()
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
