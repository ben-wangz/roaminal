package monitor

import (
	"context"
	"math"
	"sync"
	"time"

	systemclock "github.com/ben-wangz/roaminal/backend/internal/clock"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
	"github.com/ben-wangz/roaminal/backend/internal/random"
)

const (
	remoteMonitorCacheTTL = 4 * time.Second
	remoteMonitorStaleAge = 15 * time.Second
)

// RemoteMonitorService owns caching, freshness, retry serialization, and the
// bounded probe pool independently from connection-instance lifecycle.
type RemoteMonitorService struct {
	probe  ports.MonitorProbe
	clock  ports.Clock
	random ports.RandomSource
	mu     sync.Mutex
	states map[string]*remoteMonitorState
	sem    chan struct{}
}

type Dependencies struct {
	Clock  ports.Clock
	Random ports.RandomSource
}

func NewRemoteMonitorService(probe ports.MonitorProbe, dependencies ...Dependencies) *RemoteMonitorService {
	deps := Dependencies{Clock: systemclock.System{}, Random: random.CryptoSource{}}
	if len(dependencies) > 0 {
		if dependencies[0].Clock != nil {
			deps.Clock = dependencies[0].Clock
		}
		if dependencies[0].Random != nil {
			deps.Random = dependencies[0].Random
		}
	}
	return &RemoteMonitorService{probe: probe, clock: deps.Clock, random: deps.Random, states: make(map[string]*remoteMonitorState), sem: make(chan struct{}, 4)}
}

func (s *RemoteMonitorService) RemoteMonitor(ctx context.Context, id string) (RemoteMonitorSnapshot, error) {
	if s == nil || s.probe == nil {
		return RemoteMonitorSnapshot{}, ports.ErrRemoteNoTransport
	}
	if ctx == nil {
		ctx = context.Background()
	}
	target, err := s.probe.Target(ctx, id)
	if err != nil {
		return RemoteMonitorSnapshot{}, err
	}
	ownerID := target.OwnerID
	for {
		now := s.clock.Now()
		s.mu.Lock()
		state := s.states[ownerID]
		if state == nil {
			state = &remoteMonitorState{}
			s.states[ownerID] = state
		}
		if state.snapshot != nil && now.Sub(state.lastGood) < remoteMonitorCacheTTL {
			result := remoteSnapshotForNow(*state.snapshot, now, state.failures)
			s.mu.Unlock()
			return result, nil
		}
		if state.inflight != nil {
			wait := state.inflight
			s.mu.Unlock()
			select {
			case <-wait:
				continue
			case <-ctx.Done():
				return RemoteMonitorSnapshot{}, ctx.Err()
			}
		}
		inflight := make(chan struct{})
		state.inflight = inflight
		s.mu.Unlock()
		return s.probeRemoteMonitor(ctx, ownerID, target, state, inflight)
	}
}

func (s *RemoteMonitorService) probeRemoteMonitor(ctx context.Context, ownerID string, target ports.MonitorTarget, state *remoteMonitorState, inflight chan struct{}) (RemoteMonitorSnapshot, error) {
	finish := func() {
		s.mu.Lock()
		if current := s.states[ownerID]; current == state && current.inflight == inflight {
			current.inflight = nil
			close(inflight)
		}
		s.mu.Unlock()
	}
	if !s.acquire(ctx) {
		s.mu.Lock()
		result := remoteUnavailableOrCached(state, s.clock.Now())
		finishNeeded := state.inflight == inflight
		s.mu.Unlock()
		if finishNeeded {
			finish()
		}
		return result, nil
	}
	defer s.release()

	started := s.clock.Now()
	nonce := monitorNonce(s.random, started)
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	probeResult, err := s.probe.Probe(probeCtx, target, ports.MonitorProbeRequest{Script: remoteCollectorScript, Nonce: nonce})
	output := probeResult.Output
	rtt := s.clock.Since(started).Milliseconds()
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
			s.mu.Lock()
			now := s.clock.Now().UTC()
			result := buildRemoteSnapshot(state, raw, now, rtt)
			state.snapshot = &result
			state.lastGood = now
			state.failures = 0
			s.mu.Unlock()
			finish()
			return result, nil
		}
	}
	s.mu.Lock()
	state.failures++
	result := remoteUnavailableOrCached(state, s.clock.Now())
	s.mu.Unlock()
	finish()
	return result, nil
}

func (s *RemoteMonitorService) acquire(ctx context.Context) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case s.sem <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (s *RemoteMonitorService) release() { <-s.sem }

func (s *RemoteMonitorService) Clear(ownerID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if state := s.states[ownerID]; state != nil && state.inflight != nil {
		close(state.inflight)
		state.inflight = nil
	}
	delete(s.states, ownerID)
	s.mu.Unlock()
}

func (s *RemoteMonitorService) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	for ownerID, state := range s.states {
		if state != nil && state.inflight != nil {
			close(state.inflight)
		}
		delete(s.states, ownerID)
	}
	s.mu.Unlock()
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
