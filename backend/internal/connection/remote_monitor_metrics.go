package connection

import (
	"math"
	"time"
)

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
	available, warming := 0, false
	for _, status := range []string{result.Metrics.CPU.Status, result.Metrics.Memory.Status, result.Metrics.Uptime.Status, result.Metrics.Load.Status, result.Metrics.Disk.Status} {
		if status == "available" {
			available++
		}
		if status == "warming" {
			warming = true
		}
	}
	switch {
	case available == 5:
		result.Status = "available"
	case available > 0 || warming:
		result.Status = "partial"
	default:
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
				if cores >= 0 && !math.IsInf(cores, 0) {
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
	metric.Scope, metric.Status = "host", "warming"
	if previous != nil && previous.scope == "host" && current.hostTotal >= previous.hostTotal && current.hostIdle >= previous.hostIdle {
		totalDelta := current.hostTotal - previous.hostTotal
		busyDelta := totalDelta - minUint64(current.hostIdle-previous.hostIdle, totalDelta)
		if totalDelta > 0 {
			percent := float64(busyDelta) / float64(totalDelta) * 100
			metric.Percent, metric.Status = &percent, "available"
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
	metric.Percent, metric.Status = &percent, "available"
	return metric
}

func uptimeMetric(raw remoteRawSample) RemoteUptimeMetric {
	metric := RemoteUptimeMetric{Status: "unavailable", Scope: "pid1"}
	if raw.pid1StartTicks == nil || raw.clockTicks == nil || *raw.clockTicks == 0 || raw.systemUptimeSeconds == nil {
		return metric
	}
	seconds := *raw.systemUptimeSeconds - float64(*raw.pid1StartTicks)/float64(*raw.clockTicks)
	if seconds >= 0 && seconds <= 1e12 && !math.IsNaN(seconds) && !math.IsInf(seconds, 0) {
		metric.Seconds, metric.Status = &seconds, "available"
	}
	return metric
}

func loadMetric(raw remoteRawSample) RemoteLoadMetric {
	metric := RemoteLoadMetric{Status: "unavailable", Scope: "system"}
	if raw.loadOne != nil && raw.loadFive != nil && raw.loadFifteen != nil {
		metric.One, metric.Five, metric.Fifteen, metric.Status = raw.loadOne, raw.loadFive, raw.loadFifteen, "available"
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
