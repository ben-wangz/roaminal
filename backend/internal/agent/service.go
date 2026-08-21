package agent

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/config"
	"github.com/ben-wangz/roaminal/backend/internal/connection"
	"github.com/ben-wangz/roaminal/backend/internal/terminal"
)

type Service struct {
	cfg            config.Config
	terms          *connection.Manager
	store          *Store
	mu             sync.Mutex
	bindings       map[string]Target
	operations     map[string]*Initialization
	endpointOps    map[string]string
	endpointLock   map[string]*sync.Mutex
	rate           map[string]eventRate
	runtime        map[string]TargetState
	runtimeTargets map[string]string
	completedTools map[string]map[string]time.Time
	eventIDs       map[string]map[string]time.Time
	endpointCache  map[string]endpointCacheEntry
}

type eventRate struct {
	LastTokens time.Time
	Tokens     float64
}

type endpointCacheEntry struct {
	Alias       string
	SourceState string
	Key         string
	ExpiresAt   time.Time
}

func New(cfg config.Config, stateRoot string, terms *connection.Manager) *Service {
	return &Service{
		cfg: cfg, terms: terms, store: OpenStore(stateRoot), bindings: map[string]Target{},
		operations: map[string]*Initialization{}, endpointOps: map[string]string{}, endpointLock: map[string]*sync.Mutex{},
		rate: map[string]eventRate{}, runtime: map[string]TargetState{}, runtimeTargets: map[string]string{}, completedTools: map[string]map[string]time.Time{}, eventIDs: map[string]map[string]time.Time{}, endpointCache: map[string]endpointCacheEntry{},
	}
}

func (s *Service) Summary(summary connection.Summary) terminal.AgentSummary {
	result := terminal.AgentSummary{
		AgentType: "codex", Support: "unsupported", Component: "uninitialized",
		Activity: "unknown", ActivityLabel: "Codex status unknown",
	}
	if summary.Type != "ssh" {
		result.SupportReason = "local_connection"
		return result
	}
	if !summary.TmuxEnabled || summary.TmuxSessionName == "" {
		result.SupportReason = "tmux_disabled"
		return result
	}
	if summary.Lifecycle != "live" {
		result.SupportReason = "connection_not_live"
		return result
	}
	if s.terms == nil {
		result.SupportReason = "ssh_transport_unavailable"
		return result
	}
	if _, err := s.terms.RemoteTransferInfo(summary.ID); err != nil {
		result.SupportReason = "ssh_transport_unavailable"
		return result
	}
	result.Support = "supported"
	if s.store.Err() != nil {
		result.Component = "error"
		result.ErrorCode = "agent_store_unavailable"
		result.ErrorMessage = "Agent state storage is unavailable."
		return result
	}
	target, record, ok := s.targetFor(summary)
	if !ok {
		return result
	}
	result.Component = record.InstallationState
	if result.Component == "" {
		result.Component = "uninitialized"
	}
	result.ComponentVersion = record.ComponentVersion
	if state, exists := s.runtimeState(target); exists {
		result = summaryFromState(result, state)
	} else if state, exists := record.Targets[target.SessionName]; exists {
		result = summaryFromState(result, state)
	}
	if result.Component == "initializing" && !s.endpointOperationRunning(target.EndpointKey) {
		result.Component = "error"
		result.InitializationID = ""
		result.ErrorCode = "agent_initialization_interrupted"
		result.ErrorMessage = "The previous Agent initialization was interrupted and can be repaired."
	}
	return result
}

func (s *Service) runtimeState(target Target) (TargetState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.runtime[target.EndpointKey+"\x00"+target.SessionName]
	return state, ok
}

func (s *Service) endpointOperationRunning(endpointKey string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	operationID := s.endpointOps[endpointKey]
	operation := s.operations[operationID]
	return operation != nil && operation.Status == "running"
}

func summaryFromState(result terminal.AgentSummary, state TargetState) terminal.AgentSummary {
	result.Component = state.Component
	if result.Component == "" {
		result.Component = "uninitialized"
	}
	result.ComponentVersion = state.ComponentVersion
	result.Activity = state.Activity
	result.ActivityLabel = activityLabel(state.Activity)
	result.LastEventName = state.LastEventName
	if !state.LastEventAt.IsZero() {
		result.LastEventAt = state.LastEventAt.UTC().Format(time.RFC3339Nano)
	}
	result.InitializationID = state.InitializationID
	result.ErrorCode = state.ErrorCode
	result.ErrorMessage = state.ErrorMessage
	if stale := staleActivity(state); stale != "" {
		result.Activity = stale
		result.ActivityLabel = activityLabel(stale)
	}
	return result
}

func activityLabel(value string) string {
	switch value {
	case "running":
		return "Codex running"
	case "waiting":
		return "Codex waiting for permission"
	case "completed":
		return "Codex turn finished"
	case "idle":
		return "Codex idle"
	case "stale":
		return "Codex status stale"
	default:
		return "Codex status unknown"
	}
}

func staleActivity(state TargetState) string {
	when := state.LastReceivedAt
	if when.IsZero() {
		when = state.LastEventAt
	}
	if when.IsZero() {
		return ""
	}
	age := time.Since(when)
	switch state.Activity {
	case "running", "waiting":
		if age > 2*time.Hour {
			return "stale"
		}
	case "completed":
		if age > 30*time.Minute {
			return "idle"
		}
	case "idle":
		if age > 24*time.Hour {
			return "stale"
		}
	}
	return ""
}

func (s *Service) targetFor(summary connection.Summary) (Target, EndpointRecord, bool) {
	s.mu.Lock()
	target, bound := s.bindings[summary.ID]
	s.mu.Unlock()
	if bound {
		record, ok := s.store.Get(target.EndpointKey)
		return target, record, ok
	}
	alias := ""
	if summary.SourceHostAlias != nil {
		alias = *summary.SourceHostAlias
	}
	for key, record := range s.store.Snapshot() {
		for _, candidate := range record.Aliases {
			if candidate == alias {
				target := Target{EndpointKey: key, SessionName: summary.TmuxSessionName}
				return target, record, true
			}
		}
	}
	if endpointKey, ok := s.resolveCachedEndpoint(summary); ok {
		target := Target{EndpointKey: endpointKey, SessionName: summary.TmuxSessionName}
		if record, exists := s.store.Get(endpointKey); exists {
			return target, record, true
		}
	}
	return Target{}, EndpointRecord{}, false
}

func (s *Service) resolveCachedEndpoint(summary connection.Summary) (string, bool) {
	alias := ""
	if summary.SourceHostAlias != nil {
		alias = *summary.SourceHostAlias
	}
	now := time.Now()
	s.mu.Lock()
	if s.endpointCache == nil {
		s.endpointCache = map[string]endpointCacheEntry{}
	}
	entry, cached := s.endpointCache[summary.ID]
	if cached && entry.Alias == alias && entry.SourceState == summary.SourceState && now.Before(entry.ExpiresAt) {
		key := entry.Key
		s.mu.Unlock()
		return key, key != ""
	}
	s.mu.Unlock()
	if s.terms == nil {
		return "", false
	}
	effective, err := s.terms.ResolveEndpoint(context.Background(), summary.ID)
	key := ""
	if err == nil {
		if endpoint, normalizeErr := NormalizeEndpoint(effective); normalizeErr == nil {
			key = endpoint.Key
		}
	}
	s.mu.Lock()
	s.endpointCache[summary.ID] = endpointCacheEntry{Alias: alias, SourceState: summary.SourceState, Key: key, ExpiresAt: now.Add(30 * time.Second)}
	s.mu.Unlock()
	return key, key != ""
}

func randomToken() (string, string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	return token, tokenHash(token), nil
}

func randomID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func (s *Service) allowEvent(key string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rate == nil {
		s.rate = map[string]eventRate{}
	}
	window := s.rate[key]
	if window.LastTokens.IsZero() {
		window = eventRate{LastTokens: now, Tokens: 30}
	}
	if elapsed := now.Sub(window.LastTokens).Seconds(); elapsed > 0 {
		window.Tokens = minFloat(30, window.Tokens+elapsed*2)
		window.LastTokens = now
	}
	if window.Tokens < 1 {
		s.rate[key] = window
		return false
	}
	window.Tokens--
	s.rate[key] = window
	return true
}

func minFloat(left, right float64) float64 {
	if left < right {
		return left
	}
	return right
}
