package connection

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"
)

const (
	remoteMonitorCacheTTL = 4 * time.Second
	remoteMonitorStaleAge = 15 * time.Second
)

var (
	ErrRemoteInstanceNotFound = errors.New("remote connection instance not found")
	ErrRemoteNoTransport      = errors.New("no remote transport")
)

// RemoteMonitor probes the transport behind a live SSH connection. Results are
// cached per transport, so derived instances share one remote sample.
func (m *Manager) RemoteMonitor(ctx context.Context, id string) (RemoteMonitorSnapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	transport, err := m.remoteTransport(id)
	if err != nil {
		return RemoteMonitorSnapshot{}, err
	}
	ownerID := transport.OwnerID
	for {
		now := time.Now()
		m.remoteMu.Lock()
		state := m.remoteState[ownerID]
		if state == nil {
			state = &remoteMonitorState{}
			m.remoteState[ownerID] = state
		}
		if state.snapshot != nil && now.Sub(state.lastGood) < remoteMonitorCacheTTL {
			result := remoteSnapshotForNow(*state.snapshot, now, state.failures)
			m.remoteMu.Unlock()
			return result, nil
		}
		if state.inflight != nil {
			wait := state.inflight
			m.remoteMu.Unlock()
			select {
			case <-wait:
				continue
			case <-ctx.Done():
				return RemoteMonitorSnapshot{}, ctx.Err()
			}
		}
		inflight := make(chan struct{})
		state.inflight = inflight
		m.remoteMu.Unlock()
		return m.probeRemoteMonitor(ctx, ownerID, transport, state, inflight)
	}
}

func (m *Manager) remoteTransport(id string) (*Transport, error) {
	var summary Summary
	for _, item := range m.Summaries() {
		if item.ID == id {
			summary = item
			break
		}
	}
	if summary.ID == "" {
		return nil, ErrRemoteInstanceNotFound
	}
	if summary.Type != "ssh" || summary.Lifecycle != "live" {
		return nil, ErrRemoteNoTransport
	}
	m.transportMu.Lock()
	transport := m.instances[id]
	if transport != nil && transport.OwnerID != id {
		transport = m.transports[transport.OwnerID]
	}
	if !transportAcceptsReuse(transport) {
		transport = nil
	}
	m.transportMu.Unlock()
	if transport == nil {
		return nil, ErrRemoteNoTransport
	}
	return transport, nil
}

func (m *Manager) probeRemoteMonitor(ctx context.Context, ownerID string, transport *Transport, state *remoteMonitorState, inflight chan struct{}) (RemoteMonitorSnapshot, error) {
	finish := func() {
		m.remoteMu.Lock()
		if current := m.remoteState[ownerID]; current == state && current.inflight == inflight {
			current.inflight = nil
			close(inflight)
		}
		m.remoteMu.Unlock()
	}
	if !m.acquireRemoteProbe() {
		m.remoteMu.Lock()
		result := remoteUnavailableOrCached(state, time.Now())
		finishNeeded := state.inflight == inflight
		m.remoteMu.Unlock()
		if finishNeeded {
			finish()
		}
		return result, nil
	}
	defer m.releaseRemoteProbe()

	started := time.Now()
	nonce := monitorNonce()
	probeCtx, cancel := withAuxiliaryTimeout(ctx)
	output, err := m.runAuxiliaryInput(probeCtx, transport, strings.NewReader(remoteCollectorScript), "sh", "-s", "--", nonce)
	cancel()
	rtt := time.Since(started).Milliseconds()
	if rtt < 0 {
		rtt = 0
	}
	if rtt > math.MaxInt64 {
		rtt = math.MaxInt64
	}
	if err == nil {
		var raw remoteRawSample
		raw, err = parseRemoteCollector(output, nonce)
		if err == nil {
			m.remoteMu.Lock()
			result := buildRemoteSnapshot(state, raw, time.Now().UTC(), rtt)
			state.snapshot = &result
			state.lastGood = time.Now()
			state.failures = 0
			m.remoteMu.Unlock()
			finish()
			return result, nil
		}
	}
	m.remoteMu.Lock()
	state.failures++
	result := remoteUnavailableOrCached(state, time.Now())
	m.remoteMu.Unlock()
	finish()
	return result, nil
}

func (m *Manager) acquireRemoteProbe() bool {
	select {
	case m.remoteSem <- struct{}{}:
		return true
	default:
		return false
	}
}

func (m *Manager) releaseRemoteProbe() { <-m.remoteSem }

func (m *Manager) clearRemoteState(ownerID string) {
	m.remoteMu.Lock()
	if state := m.remoteState[ownerID]; state != nil && state.inflight != nil {
		close(state.inflight)
		state.inflight = nil
	}
	delete(m.remoteState, ownerID)
	m.remoteMu.Unlock()
}

func remoteUnavailableOrCached(state *remoteMonitorState, now time.Time) RemoteMonitorSnapshot {
	if state.snapshot == nil {
		return emptyRemoteSnapshot()
	}
	return remoteSnapshotForNow(*state.snapshot, now, state.failures)
}

func remoteSnapshotForNow(snapshot RemoteMonitorSnapshot, now time.Time, failures int) RemoteMonitorSnapshot {
	if snapshot.SampledAt == nil {
		return snapshot
	}
	age := now.Sub(snapshot.SampledAt.In(time.UTC)).Milliseconds()
	if age < 0 {
		age = 0
	}
	snapshot.AgeMs = &age
	if failures >= 3 || age > remoteMonitorStaleAge.Milliseconds() {
		snapshot.Status = "stale"
		if failures >= 3 {
			snapshot.Status = "unavailable"
		}
	}
	return snapshot
}

func emptyRemoteSnapshot() RemoteMonitorSnapshot {
	return RemoteMonitorSnapshot{Status: "unavailable", Metrics: RemoteMonitorMetrics{
		CPU:    RemoteCPUMetric{Status: "unavailable", Scope: "unknown"},
		Memory: RemoteMemoryMetric{Status: "unavailable", Scope: "unknown"},
		Uptime: RemoteUptimeMetric{Status: "unavailable", Scope: "pid1"},
		Load:   RemoteLoadMetric{Status: "unavailable", Scope: "system"},
		Disk:   RemoteDiskMetric{Status: "unavailable", Scope: "rootfs", Mount: "/"},
	}}
}
