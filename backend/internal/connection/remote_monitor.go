package connection

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"math"
	"strconv"
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

type RemoteMonitorSnapshot struct {
	Status     string               `json:"status"`
	SampledAt  *time.Time           `json:"sampledAt"`
	AgeMs      *int64               `json:"ageMs"`
	Metrics    RemoteMonitorMetrics `json:"metrics"`
	ProbeRttMs *int64               `json:"probeRttMs"`
}

type RemoteMonitorMetrics struct {
	CPU    RemoteCPUMetric    `json:"cpu"`
	Memory RemoteMemoryMetric `json:"memory"`
	Uptime RemoteUptimeMetric `json:"uptime"`
	Load   RemoteLoadMetric   `json:"load"`
	Disk   RemoteDiskMetric   `json:"disk"`
}

type RemoteCPUMetric struct {
	Status        string   `json:"status"`
	Scope         string   `json:"scope"`
	Percent       *float64 `json:"percent"`
	UsageCores    *float64 `json:"usageCores"`
	CapacityCores *float64 `json:"capacityCores"`
}

type RemoteMemoryMetric struct {
	Status          string   `json:"status"`
	Scope           string   `json:"scope"`
	WorkingSetBytes *int64   `json:"workingSetBytes"`
	CurrentBytes    *int64   `json:"currentBytes"`
	LimitBytes      *int64   `json:"limitBytes"`
	Percent         *float64 `json:"percent"`
}

type RemoteUptimeMetric struct {
	Status  string   `json:"status"`
	Scope   string   `json:"scope"`
	Seconds *float64 `json:"seconds"`
}

type RemoteLoadMetric struct {
	Status  string   `json:"status"`
	Scope   string   `json:"scope"`
	One     *float64 `json:"one"`
	Five    *float64 `json:"five"`
	Fifteen *float64 `json:"fifteen"`
}

type RemoteDiskMetric struct {
	Status         string   `json:"status"`
	Scope          string   `json:"scope"`
	Mount          string   `json:"mount"`
	TotalBytes     *int64   `json:"totalBytes"`
	UsedBytes      *int64   `json:"usedBytes"`
	AvailableBytes *int64   `json:"availableBytes"`
	Percent        *float64 `json:"percent"`
}

type remoteMonitorState struct {
	snapshot *RemoteMonitorSnapshot
	baseline *remoteCPUBaseline
	lastGood time.Time
	failures int
	inflight chan struct{}
}

type remoteCPUBaseline struct {
	scope     string
	usageNS   uint64
	hostTotal uint64
	hostIdle  uint64
	sampledAt time.Time
}

type remoteRawSample struct {
	scope                    string
	cpuUsageNS               *uint64
	cpuCapacityMilli         *uint64
	hostCPUTotalTicks        *uint64
	hostCPUIdleTicks         *uint64
	memoryCurrentBytes       *uint64
	memoryInactiveBytes      *uint64
	memoryLimitBytes         *uint64
	hostMemoryTotalBytes     *uint64
	hostMemoryAvailableBytes *uint64
	pid1StartTicks           *uint64
	clockTicks               *uint64
	systemUptimeSeconds      *float64
	loadOne                  *float64
	loadFive                 *float64
	loadFifteen              *float64
	rootfsTotalKiB           *uint64
	rootfsUsedKiB            *uint64
	rootfsAvailableKiB       *uint64
	rootfsPercent            *float64
}

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

func buildRemoteSnapshot(state *remoteMonitorState, raw remoteRawSample, sampledAt time.Time, rtt int64) RemoteMonitorSnapshot {
	result := emptyRemoteSnapshot()
	result.SampledAt = &sampledAt
	result.AgeMs = pointerInt64(0)
	result.ProbeRttMs = pointerInt64(rtt)
	result.Metrics.Memory = memoryMetric(raw)
	result.Metrics.Uptime = uptimeMetric(raw)
	result.Metrics.Load = loadMetric(raw)
	result.Metrics.Disk = diskMetric(raw)
	result.Metrics.CPU, state.baseline = cpuMetric(state.baseline, raw, sampledAt)
	available := 0
	warming := false
	for _, status := range []string{result.Metrics.CPU.Status, result.Metrics.Memory.Status, result.Metrics.Uptime.Status, result.Metrics.Load.Status, result.Metrics.Disk.Status} {
		if status == "available" {
			available++
		}
		if status == "warming" {
			warming = true
		}
	}
	if available == 5 {
		result.Status = "available"
	} else if available > 0 || warming {
		result.Status = "partial"
	} else {
		result.Status = "unavailable"
	}
	return result
}

func cpuMetric(previous *remoteCPUBaseline, raw remoteRawSample, at time.Time) (RemoteCPUMetric, *remoteCPUBaseline) {
	metric := RemoteCPUMetric{Status: "unavailable", Scope: raw.scope}
	if metric.Scope == "" {
		metric.Scope = "unknown"
	}
	current := &remoteCPUBaseline{scope: raw.scope, sampledAt: at}
	if raw.cpuUsageNS != nil {
		current.usageNS = *raw.cpuUsageNS
		metric.Status = "warming"
		if raw.cpuCapacityMilli != nil && *raw.cpuCapacityMilli > 0 {
			capacity := float64(*raw.cpuCapacityMilli) / 1000
			metric.CapacityCores = &capacity
		}
		if previous != nil && previous.scope == current.scope && current.usageNS >= previous.usageNS {
			elapsed := at.Sub(previous.sampledAt).Seconds()
			if elapsed > 0 && elapsed <= 120 {
				cores := float64(current.usageNS-previous.usageNS) / 1e9 / elapsed
				if cores >= 0 && math.IsInf(cores, 0) == false {
					metric.UsageCores = &cores
					if metric.CapacityCores != nil && *metric.CapacityCores > 0 {
						percent := cores / *metric.CapacityCores * 100
						if percent >= 0 && percent <= 10000 {
							metric.Percent = &percent
						}
					}
					metric.Status = "available"
				}
			}
		}
		return metric, current
	}
	if raw.hostCPUTotalTicks == nil || raw.hostCPUIdleTicks == nil {
		return metric, current
	}
	current.hostTotal, current.hostIdle = *raw.hostCPUTotalTicks, *raw.hostCPUIdleTicks
	metric.Scope = "host"
	metric.Status = "warming"
	if previous != nil && previous.scope == "host" && current.hostTotal >= previous.hostTotal && current.hostIdle >= previous.hostIdle {
		totalDelta := current.hostTotal - previous.hostTotal
		busyDelta := totalDelta - minUint64(current.hostIdle-previous.hostIdle, totalDelta)
		if totalDelta > 0 {
			percent := float64(busyDelta) / float64(totalDelta) * 100
			metric.Percent = &percent
			metric.Status = "available"
		}
	}
	return metric, current
}

func memoryMetric(raw remoteRawSample) RemoteMemoryMetric {
	metric := RemoteMemoryMetric{Status: "unavailable", Scope: raw.scope}
	if metric.Scope == "" {
		metric.Scope = "unknown"
	}
	if raw.memoryCurrentBytes != nil {
		current := safeInt64(*raw.memoryCurrentBytes)
		metric.CurrentBytes = &current
		working := *raw.memoryCurrentBytes
		if raw.memoryInactiveBytes != nil && *raw.memoryInactiveBytes < working {
			working -= *raw.memoryInactiveBytes
		} else if raw.memoryInactiveBytes != nil {
			working = 0
		}
		workingValue := safeInt64(working)
		metric.WorkingSetBytes = &workingValue
		if raw.memoryLimitBytes != nil && *raw.memoryLimitBytes > 0 {
			limit := safeInt64(*raw.memoryLimitBytes)
			metric.LimitBytes = &limit
			percent := float64(working) / float64(*raw.memoryLimitBytes) * 100
			if percent >= 0 && percent <= 10000 {
				metric.Percent = &percent
			}
		}
		metric.Status = "available"
		return metric
	}
	if raw.hostMemoryTotalBytes == nil || raw.hostMemoryAvailableBytes == nil || *raw.hostMemoryAvailableBytes > *raw.hostMemoryTotalBytes {
		metric.Scope = "unknown"
		return metric
	}
	metric.Scope = "host"
	used := *raw.hostMemoryTotalBytes - *raw.hostMemoryAvailableBytes
	total, working := safeInt64(*raw.hostMemoryTotalBytes), safeInt64(used)
	metric.CurrentBytes, metric.WorkingSetBytes, metric.LimitBytes = &working, &working, &total
	percent := float64(used) / float64(*raw.hostMemoryTotalBytes) * 100
	metric.Percent = &percent
	metric.Status = "available"
	return metric
}

func uptimeMetric(raw remoteRawSample) RemoteUptimeMetric {
	metric := RemoteUptimeMetric{Status: "unavailable", Scope: "pid1"}
	if raw.pid1StartTicks == nil || raw.clockTicks == nil || *raw.clockTicks == 0 || raw.systemUptimeSeconds == nil {
		return metric
	}
	seconds := *raw.systemUptimeSeconds - float64(*raw.pid1StartTicks)/float64(*raw.clockTicks)
	if seconds >= 0 && seconds <= 1e12 && !math.IsNaN(seconds) && !math.IsInf(seconds, 0) {
		metric.Seconds = &seconds
		metric.Status = "available"
	}
	return metric
}

func loadMetric(raw remoteRawSample) RemoteLoadMetric {
	metric := RemoteLoadMetric{Status: "unavailable", Scope: "system"}
	if raw.loadOne != nil && raw.loadFive != nil && raw.loadFifteen != nil {
		metric.One, metric.Five, metric.Fifteen = raw.loadOne, raw.loadFive, raw.loadFifteen
		metric.Status = "available"
	}
	return metric
}

func diskMetric(raw remoteRawSample) RemoteDiskMetric {
	metric := RemoteDiskMetric{Status: "unavailable", Scope: "rootfs", Mount: "/"}
	if raw.rootfsTotalKiB == nil || raw.rootfsUsedKiB == nil || raw.rootfsAvailableKiB == nil || *raw.rootfsUsedKiB > *raw.rootfsTotalKiB {
		return metric
	}
	const maxKiB = uint64(math.MaxInt64) / 1024
	if *raw.rootfsTotalKiB > maxKiB || *raw.rootfsUsedKiB > maxKiB || *raw.rootfsAvailableKiB > maxKiB {
		return metric
	}
	total, used, available := int64(*raw.rootfsTotalKiB*1024), int64(*raw.rootfsUsedKiB*1024), int64(*raw.rootfsAvailableKiB*1024)
	metric.TotalBytes, metric.UsedBytes, metric.AvailableBytes = &total, &used, &available
	if raw.rootfsPercent != nil && *raw.rootfsPercent >= 0 && *raw.rootfsPercent <= 100 {
		metric.Percent = raw.rootfsPercent
	}
	metric.Status = "available"
	return metric
}

func monitorNonce() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(data[:])
}

func pointerInt64(value int64) *int64 { return &value }
func safeInt64(value uint64) int64 {
	if value > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(value)
}
func minUint64(left, right uint64) uint64 {
	if left < right {
		return left
	}
	return right
}

func parseRemoteCollector(output []byte, nonce string) (remoteRawSample, error) {
	var result remoteRawSample
	allowed := map[string]bool{
		"scope": true, "cpu_usage_ns": true, "cpu_capacity_milli": true, "host_cpu_total_ticks": true, "host_cpu_idle_ticks": true,
		"memory_current_bytes": true, "memory_inactive_file_bytes": true, "memory_limit_bytes": true, "host_memory_total_bytes": true, "host_memory_available_bytes": true,
		"pid1_start_ticks": true, "clock_ticks_per_second": true, "system_uptime_seconds": true, "load_1": true, "load_5": true, "load_15": true,
		"rootfs_total_kib": true, "rootfs_used_kib": true, "rootfs_available_kib": true, "rootfs_capacity_percent": true,
	}
	seen := map[string]bool{}
	begin, end := "ROAMINAL_MONITOR_V1_BEGIN_"+nonce, "ROAMINAL_MONITOR_V1_END_"+nonce
	inside, foundBegin, foundEnd := false, false, false
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == begin {
			if inside || foundBegin {
				return result, errors.New("duplicate monitor begin marker")
			}
			inside, foundBegin = true, true
			continue
		}
		if line == end {
			if !inside || foundEnd {
				return result, errors.New("invalid monitor end marker")
			}
			inside, foundEnd = false, true
			continue
		}
		if !inside {
			continue
		}
		if line == "" || !strings.Contains(line, "=") {
			return result, errors.New("invalid monitor field")
		}
		parts := strings.SplitN(line, "=", 2)
		key, value := parts[0], parts[1]
		if !allowed[key] || seen[key] {
			return result, errors.New("unknown or duplicate monitor field")
		}
		seen[key] = true
		switch key {
		case "scope":
			if value != "cgroup-v1" && value != "cgroup-v2" && value != "host" && value != "unknown" {
				return result, errors.New("invalid monitor scope")
			}
			result.scope = value
		case "system_uptime_seconds":
			parsed, err := parseFixedDecimal(value)
			if err != nil {
				return result, err
			}
			result.systemUptimeSeconds = &parsed
		case "load_1", "load_5", "load_15", "rootfs_capacity_percent":
			parsed, err := parseFixedDecimal(value)
			if err != nil || parsed < 0 || parsed > 1e12 {
				return result, errors.New("invalid monitor decimal")
			}
			switch key {
			case "load_1":
				result.loadOne = &parsed
			case "load_5":
				result.loadFive = &parsed
			case "load_15":
				result.loadFifteen = &parsed
			default:
				result.rootfsPercent = &parsed
			}
		default:
			parsed, err := parseBoundedUint(value)
			if err != nil {
				return result, err
			}
			switch key {
			case "cpu_usage_ns":
				result.cpuUsageNS = &parsed
			case "cpu_capacity_milli":
				result.cpuCapacityMilli = &parsed
			case "host_cpu_total_ticks":
				result.hostCPUTotalTicks = &parsed
			case "host_cpu_idle_ticks":
				result.hostCPUIdleTicks = &parsed
			case "memory_current_bytes":
				result.memoryCurrentBytes = &parsed
			case "memory_inactive_file_bytes":
				result.memoryInactiveBytes = &parsed
			case "memory_limit_bytes":
				result.memoryLimitBytes = &parsed
			case "host_memory_total_bytes":
				result.hostMemoryTotalBytes = &parsed
			case "host_memory_available_bytes":
				result.hostMemoryAvailableBytes = &parsed
			case "pid1_start_ticks":
				result.pid1StartTicks = &parsed
			case "clock_ticks_per_second":
				result.clockTicks = &parsed
			case "rootfs_total_kib":
				result.rootfsTotalKiB = &parsed
			case "rootfs_used_kib":
				result.rootfsUsedKiB = &parsed
			case "rootfs_available_kib":
				result.rootfsAvailableKiB = &parsed
			}
		}
	}
	if !foundBegin || !foundEnd || inside || result.scope == "" {
		return result, errors.New("monitor markers incomplete")
	}
	return result, nil
}

func parseBoundedUint(value string) (uint64, error) {
	if value == "" || strings.HasPrefix(value, "+") || strings.ContainsAny(value, ".eE- 	\r\n") {
		return 0, errors.New("invalid monitor integer")
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil || parsed > 1<<62 {
		return 0, errors.New("monitor integer out of range")
	}
	return parsed, nil
}

func parseFixedDecimal(value string) (float64, error) {
	if value == "" || strings.ContainsAny(value, "eE+ \t\r\n") {
		return 0, errors.New("invalid monitor decimal")
	}
	for index, char := range value {
		if (char < '0' || char > '9') && char != '.' && !(char == '-' && index == 0) {
			return 0, errors.New("invalid monitor decimal")
		}
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) || parsed < 0 {
		return 0, errors.New("invalid monitor decimal")
	}
	return parsed, nil
}

const remoteCollectorScript = `#!/bin/sh
set -u
nonce=$1
printf 'ROAMINAL_MONITOR_V1_BEGIN_%s\n' "$nonce"
scope=unknown
cg=/sys/fs/cgroup
has_cgroup=0
grep -qE '^[^:]+:[^:]+:' /proc/self/cgroup 2>/dev/null && has_cgroup=1
v2info=$(awk -F' - ' '$2 ~ /^cgroup2[[:space:]]/ {print $1; exit}' /proc/self/mountinfo 2>/dev/null || true)
v2root=$(printf '%s\n' "$v2info" | awk '{print $4}')
v2mount=$(printf '%s\n' "$v2info" | awk '{print $5}')
[ -n "$v2mount" ] && cg=$v2mount
if [ -r "$cg/cpu.stat" ] && [ -r "$cg/memory.current" ]; then
  path=$(awk -F: '$1 == "0" {print $3; exit}' /proc/self/cgroup 2>/dev/null || true)
  if [ -n "$v2root" ] && [ "$v2root" != "/" ]; then
    case "$path" in ("$v2root"|"$v2root"/*) path=${path#"$v2root"};; (*) path=;; esac
  fi
  case "$path" in (/*..*) path=;; esac
  [ -n "$path" ] && [ "$path" != "/" ] && cg="$cg$path"
  usage=$(awk '$1 == "usage_usec" {print $2; exit}' "$cg/cpu.stat" 2>/dev/null || true)
  current=$(cat "$cg/memory.current" 2>/dev/null || true)
  if [ -n "$usage" ] && [ -n "$current" ]; then
    scope=cgroup-v2
    printf 'cpu_usage_ns=%s\n' "$((usage * 1000))"
    max=$(awk '{print $1}' "$cg/cpu.max" 2>/dev/null || true)
    period=$(awk '{print $2}' "$cg/cpu.max" 2>/dev/null || true)
    cpuset=$(cat "$cg/cpuset.cpus.effective" 2>/dev/null || true)
    set_capacity=$(printf '%s\n' "$cpuset" | awk -F, 'NF {for (i=1; i<=NF; i++) {split($i, r, "-"); if (r[2] != "") n += r[2] - r[1] + 1; else n++}} END {if (n > 0) print n * 1000}' 2>/dev/null || true)
    case "$max:$period" in
      (max:*|'':*) [ -n "$set_capacity" ] && printf 'cpu_capacity_milli=%s\n' "$set_capacity";;
      (*:0|'':0) [ -n "$set_capacity" ] && printf 'cpu_capacity_milli=%s\n' "$set_capacity";;
      (*) quota_capacity=$((max * 1000 / period)); if [ -n "$set_capacity" ] && [ "$set_capacity" -lt "$quota_capacity" ]; then quota_capacity=$set_capacity; fi; printf 'cpu_capacity_milli=%s\n' "$quota_capacity";;
    esac
    printf 'memory_current_bytes=%s\n' "$current"
    inactive=$(awk '$1 == "inactive_file" {print $2; exit}' "$cg/memory.stat" 2>/dev/null || true)
    [ -n "$inactive" ] && printf 'memory_inactive_file_bytes=%s\n' "$inactive"
    limit=$(cat "$cg/memory.max" 2>/dev/null || true)
    case "$limit" in (''|max) ;; (*) printf 'memory_limit_bytes=%s\n' "$limit";; esac
  fi
fi
if [ "$scope" = unknown ] && [ "$has_cgroup" = 1 ]; then
  cpu_info=$(awk -F' - ' '$2 ~ /^cgroup[[:space:]]/ && $2 ~ /(^|,)cpuacct(,|[[:space:]])/ {print $1; exit}' /proc/self/mountinfo 2>/dev/null || true)
  cpu_root=$(printf '%s\n' "$cpu_info" | awk '{print $4}')
  cpu_mount=$(printf '%s\n' "$cpu_info" | awk '{print $5}')
  [ -z "$cpu_mount" ] && cpu_mount=/sys/fs/cgroup/cpu,cpuacct
  memory_info=$(awk -F' - ' '$2 ~ /^cgroup[[:space:]]/ && $2 ~ /(^|,)memory(,|[[:space:]])/ {print $1; exit}' /proc/self/mountinfo 2>/dev/null || true)
  memory_root=$(printf '%s\n' "$memory_info" | awk '{print $4}')
  memory_mount=$(printf '%s\n' "$memory_info" | awk '{print $5}')
  [ -z "$memory_mount" ] && memory_mount=/sys/fs/cgroup/memory
  cpu_path=$(awk -F: '$2 ~ /(^|,)cpuacct(,|$)/ {print $3; exit}' /proc/self/cgroup 2>/dev/null || true)
  memory_path=$(awk -F: '$2 ~ /(^|,)memory(,|$)/ {print $3; exit}' /proc/self/cgroup 2>/dev/null || true)
  case "$cpu_path:$memory_path" in (*..*) cpu_path=; memory_path=;; esac
  if [ -n "$cpu_root" ] && [ "$cpu_root" != "/" ]; then case "$cpu_path" in ("$cpu_root"|"$cpu_root"/*) cpu_path=${cpu_path#"$cpu_root"};; (*) cpu_path=;; esac; fi
  if [ -n "$memory_root" ] && [ "$memory_root" != "/" ]; then case "$memory_path" in ("$memory_root"|"$memory_root"/*) memory_path=${memory_path#"$memory_root"};; (*) memory_path=;; esac; fi
  [ -n "$cpu_path" ] && [ "$cpu_path" != "/" ] && cpu_mount="$cpu_mount$cpu_path"
  [ -n "$memory_path" ] && [ "$memory_path" != "/" ] && memory_mount="$memory_mount$memory_path"
  usage=$(cat "$cpu_mount/cpuacct.usage" 2>/dev/null || true)
  current=$(cat "$memory_mount/memory.usage_in_bytes" 2>/dev/null || true)
  if [ -n "$usage" ] && [ -n "$current" ]; then
    scope=cgroup-v1
    printf 'cpu_usage_ns=%s\n' "$usage"
    quota=$(cat "$cpu_mount/cpu.cfs_quota_us" 2>/dev/null || true)
    period=$(cat "$cpu_mount/cpu.cfs_period_us" 2>/dev/null || true)
    case "$quota:$period" in (-1:*|'':*) ;; (*:0|'':0) ;; (*) printf 'cpu_capacity_milli=%s\n' "$((quota * 1000 / period))";; esac
    printf 'memory_current_bytes=%s\n' "$current"
    inactive=$(awk '$1 == "total_inactive_file" || $1 == "inactive_file" {print $2; exit}' "$memory_mount/memory.stat" 2>/dev/null || true)
    [ -n "$inactive" ] && printf 'memory_inactive_file_bytes=%s\n' "$inactive"
    limit=$(cat "$memory_mount/memory.limit_in_bytes" 2>/dev/null || true)
    case "$limit" in (''|9223372036854771712|9223372036854775807) ;; (*) printf 'memory_limit_bytes=%s\n' "$limit";; esac
  fi
fi
if [ "$scope" = unknown ] && [ "$has_cgroup" = 0 ] && [ -r /proc/stat ]; then
  line=$(awk '$1 == "cpu" {print; exit}' /proc/stat 2>/dev/null || true)
  set -- $line
  if [ "$#" -ge 5 ]; then
    total=0; i=2
    while [ "$i" -le "$#" ]; do eval "value=\${$i}"; total=$((total + value)); i=$((i + 1)); done
    idle=$(( $5 + ${6:-0} ))
    scope=host
    printf 'host_cpu_total_ticks=%s\n' "$total"
    printf 'host_cpu_idle_ticks=%s\n' "$idle"
  fi
fi
if [ "$scope" = host ] && [ -r /proc/meminfo ]; then
  total=$(awk '$1 == "MemTotal:" {print $2 * 1024; exit}' /proc/meminfo)
  available=$(awk '$1 == "MemAvailable:" {print $2 * 1024; exit}' /proc/meminfo)
  [ -n "$total" ] && [ -n "$available" ] && printf 'host_memory_total_bytes=%s\nhost_memory_available_bytes=%s\n' "$total" "$available"
fi
uptime=$(awk '{print $1; exit}' /proc/uptime 2>/dev/null || true)
stat=$(cat /proc/1/stat 2>/dev/null || true)
rest=${stat##*) }
set -- $rest
start_ticks=${20:-}
clock_ticks=$(getconf CLK_TCK 2>/dev/null || true)
[ -n "$start_ticks" ] && printf 'pid1_start_ticks=%s\n' "$start_ticks"
[ -n "$clock_ticks" ] && printf 'clock_ticks_per_second=%s\n' "$clock_ticks"
[ -n "$uptime" ] && printf 'system_uptime_seconds=%s\n' "$uptime"
load=$(cat /proc/loadavg 2>/dev/null || true)
set -- $load
[ "$#" -ge 3 ] && printf 'load_1=%s\nload_5=%s\nload_15=%s\n' "$1" "$2" "$3"
disk=$(LC_ALL=C df -k -P / 2>/dev/null | awk 'NR > 1 {print $(NF-4), $(NF-3), $(NF-2), $(NF-1); exit}')
set -- $disk
if [ "$#" -ge 4 ]; then
  percent=${4%%%}
  case "$percent" in (*[!0-9.]*) ;; (*) printf 'rootfs_total_kib=%s\nrootfs_used_kib=%s\nrootfs_available_kib=%s\nrootfs_capacity_percent=%s\n' "$1" "$2" "$3" "$percent";; esac
fi
printf 'scope=%s\n' "$scope"
printf 'ROAMINAL_MONITOR_V1_END_%s\n' "$nonce"
`
