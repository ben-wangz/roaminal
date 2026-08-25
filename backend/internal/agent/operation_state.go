package agent

import (
	"errors"
	"sync"
	"time"
)

func (s *Service) endpointMutex(key string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.endpointLock[key] == nil {
		s.endpointLock[key] = &sync.Mutex{}
	}
	return s.endpointLock[key]
}

func (s *Service) setTargetInitialization(target Target, operationID string) error {
	return s.store.Update(target.EndpointKey, func(record *EndpointRecord) error {
		state := record.Targets[target.SessionName]
		state.SessionName = target.SessionName
		state.InitializationID = operationID
		state.Component = "initializing"
		state.Activity = "unknown"
		record.Targets[target.SessionName] = state
		return nil
	})
}

func (s *Service) commitPending(endpointKey string, target Target, version, checksum string) error {
	return s.store.Update(endpointKey, func(record *EndpointRecord) error {
		record.ActiveTokenHash = record.PendingTokenHash
		record.PendingTokenHash, record.PendingCreatedAt = "", ""
		record.ComponentVersion, record.ComponentSHA256 = version, checksum
		state := record.Targets[target.SessionName]
		component := "needs_trust"
		if record.InstallationState == "ready" || state.Component == "ready" {
			component = "ready"
		}
		record.InstallationState = component
		state.Component, state.ComponentVersion, state.Activity = component, version, "unknown"
		record.Targets[target.SessionName] = state
		return nil
	})
}

func (s *Service) markTargetComponent(target Target, component, version string) error {
	return s.store.Update(target.EndpointKey, func(record *EndpointRecord) error {
		record.InstallationState = component
		record.ComponentVersion = version
		state := record.Targets[target.SessionName]
		state.SessionName = target.SessionName
		state.Component = component
		state.ComponentVersion = version
		state.Activity = "unknown"
		record.Targets[target.SessionName] = state
		return nil
	})
}

func (s *Service) completeInitialization(id string, target Target, result string, changed bool, component, version string) {
	s.mu.Lock()
	operation := s.operations[id]
	if operation != nil {
		operation.Status, operation.Result, operation.Component, operation.Changed = "completed", result, component, changed
		if version != "" {
			operation.Component = component
		}
		now := s.now().UTC()
		operation.FinishedAt = &now
		if s.endpointOps[operation.Endpoint.Key] == id {
			delete(s.endpointOps, operation.Endpoint.Key)
		}
	}
	s.mu.Unlock()
	_ = s.store.Update(target.EndpointKey, func(record *EndpointRecord) error {
		for name, state := range record.Targets {
			if name != target.SessionName && state.InitializationID != id {
				continue
			}
			if state.Component != "ready" {
				state.Component = component
				state.ComponentVersion = version
				state.Activity = "unknown"
				state.ErrorCode, state.ErrorMessage = "", ""
			}
			state.InitializationID = ""
			record.Targets[name] = state
		}
		record.UpdatedAt = s.now().UTC().Format(time.RFC3339Nano)
		return nil
	})
}

func (s *Service) failInitialization(id string, target Target, cause error) {
	code, message := "agent_install_failed", "The Agent initialization failed."
	var agentErr *Error
	if errors.As(cause, &agentErr) {
		code, message = agentErr.Code, agentErr.Message
	}
	s.mu.Lock()
	operation := s.operations[id]
	if operation != nil {
		operation.Status, operation.Component = "failed", "error"
		operation.Error = &SafeError{Code: code, Message: message}
		now := s.now().UTC()
		operation.FinishedAt = &now
		if s.endpointOps[operation.Endpoint.Key] == id {
			delete(s.endpointOps, operation.Endpoint.Key)
		}
	}
	s.mu.Unlock()
	_ = s.store.Update(target.EndpointKey, func(record *EndpointRecord) error {
		anyReady := record.InstallationState == "ready"
		for name, state := range record.Targets {
			if name != target.SessionName && state.InitializationID != id {
				continue
			}
			if state.Component != "ready" {
				state.Component, state.ErrorCode, state.ErrorMessage = "error", code, message
			} else {
				anyReady = true
			}
			state.InitializationID = ""
			record.Targets[name] = state
		}
		if !anyReady {
			record.InstallationState = "error"
		}
		record.UpdatedAt = s.now().UTC().Format(time.RFC3339Nano)
		return nil
	})
}

func (s *Service) GetInitialization(id string) (Initialization, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneOperationsLocked(s.now().UTC())
	value, ok := s.operations[id]
	if !ok || value == nil {
		return Initialization{}, false
	}
	return *value, true
}

func (s *Service) pruneOperationsLocked(now time.Time) {
	for id, operation := range s.operations {
		if operation != nil && operation.FinishedAt != nil && now.Sub(*operation.FinishedAt) > time.Hour {
			delete(s.operations, id)
		}
	}
	for len(s.operations) > 256 {
		oldestID := ""
		var oldest time.Time
		for id, operation := range s.operations {
			if operation == nil || operation.FinishedAt == nil {
				continue
			}
			if oldestID == "" || operation.FinishedAt.Before(oldest) {
				oldestID, oldest = id, *operation.FinishedAt
			}
		}
		if oldestID == "" {
			break
		}
		delete(s.operations, oldestID)
	}
}
