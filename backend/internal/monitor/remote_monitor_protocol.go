package monitor

import (
	"encoding/hex"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

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
	if value == "" || strings.HasPrefix(value, "+") || strings.ContainsAny(value, ".eE- \t\r\n") {
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

func monitorNonce(source ports.RandomSource, now time.Time) string {
	var data [16]byte
	if source != nil {
		if _, err := source.Read(data[:]); err == nil {
			return hex.EncodeToString(data[:])
		}
	}
	return strconv.FormatInt(now.UnixNano(), 16)
}
