package agent

import (
	"testing"
	"time"
)

func TestCompleteInitializationUpdatesJoinedTargets(t *testing.T) {
	service := newStoreTestService(t)
	target := Target{EndpointKey: "endpoint-test", SessionName: "first"}
	operation := &Initialization{ID: "operation-1", Endpoint: Endpoint{Key: target.EndpointKey}, Status: "running"}
	service.operations[operation.ID] = operation
	service.endpointOps[target.EndpointKey] = operation.ID
	if err := service.store.Update(target.EndpointKey, func(record *EndpointRecord) error {
		record.InstallationState = "initializing"
		record.Targets["first"] = TargetState{SessionName: "first", Component: "initializing", InitializationID: operation.ID}
		record.Targets["second"] = TargetState{SessionName: "second", Component: "initializing", InitializationID: operation.ID}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	initial, _ := service.store.Get(target.EndpointKey)
	if len(initial.Targets) != 2 {
		t.Fatalf("target states were not persisted: %+v", initial)
	}

	service.completeInitialization(operation.ID, target, "installed", true, "needs_trust", "1")
	record, ok := service.store.Get(target.EndpointKey)
	if !ok {
		t.Fatal("endpoint record was not saved")
	}
	for _, name := range []string{"first", "second"} {
		state := record.Targets[name]
		if state.Component != "needs_trust" || state.ComponentVersion != "1" || state.InitializationID != "" {
			t.Fatalf("target %q was not completed: %+v", name, state)
		}
	}
	if operation.Status != "completed" || operation.FinishedAt == nil {
		t.Fatalf("operation was not completed: %+v", operation)
	}
}

func TestFailInitializationPreservesReadyJoinedTarget(t *testing.T) {
	service := newStoreTestService(t)
	target := Target{EndpointKey: "endpoint-test", SessionName: "first"}
	operation := &Initialization{ID: "operation-1", Endpoint: Endpoint{Key: target.EndpointKey}, Status: "running"}
	service.operations[operation.ID] = operation
	service.endpointOps[target.EndpointKey] = operation.ID
	if err := service.store.Update(target.EndpointKey, func(record *EndpointRecord) error {
		record.InstallationState = "ready"
		record.Targets["first"] = TargetState{SessionName: "first", Component: "ready", InitializationID: operation.ID}
		record.Targets["second"] = TargetState{SessionName: "second", Component: "initializing", InitializationID: operation.ID}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	initial, _ := service.store.Get(target.EndpointKey)
	if len(initial.Targets) != 2 {
		t.Fatalf("target states were not persisted: %+v", initial)
	}

	service.failInitialization(operation.ID, target, errf("agent_remote_probe_failed", 502, "probe failed", nil))
	record, ok := service.store.Get(target.EndpointKey)
	if !ok {
		t.Fatal("endpoint record was not saved")
	}
	if record.InstallationState != "ready" || record.Targets["first"].Component != "ready" || record.Targets["second"].Component != "error" {
		t.Fatalf("unexpected joined failure state: %+v", record)
	}
	if record.Targets["first"].InitializationID != "" || record.Targets["second"].InitializationID != "" {
		t.Fatal("initialization IDs were not cleared")
	}
	if operation.Status != "failed" || operation.Error == nil || operation.FinishedAt.Before(time.Now().Add(-time.Minute)) {
		t.Fatalf("operation error was not recorded: %+v", operation)
	}
}
