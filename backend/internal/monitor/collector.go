package monitor

import (
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	systemclock "github.com/ben-wangz/roaminal/backend/internal/clock"
	"github.com/ben-wangz/roaminal/backend/internal/ports"
)

type Monitor struct {
	started        time.Time
	read           readFile
	now            clock
	cgroup         *cgroup
	model          string
	count          int
	minGHz, maxGHz float64
	base           SystemStats
	mu             sync.RWMutex
	current        SystemStats
	previousUsage  uint64
	previousAt     time.Time
	stop           chan struct{}
	done           chan struct{}
}

func New() *Monitor { return newMonitor(os.ReadFile, systemclock.System{}.Now) }
func NewWithClock(runtimeClock ports.Clock) *Monitor {
	if runtimeClock == nil {
		return New()
	}
	return newMonitor(os.ReadFile, runtimeClock.Now)
}
func NewWithReader(read readFile, now clock) *Monitor { return newMonitor(read, now) }
func newMonitor(read readFile, now clock) *Monitor {
	started := now()
	model, count, min, max := cpuInfo(read)
	cg, _ := discoverCgroup(read)
	m := &Monitor{started: started, read: read, now: now, cgroup: cg, model: model, count: count, minGHz: min, maxGHz: max, stop: make(chan struct{}), done: make(chan struct{})}
	m.base = SystemStats{Hostname: readFirstLine(read, "/etc/hostname"), Kernel: readFirstLine(read, "/proc/sys/kernel/osrelease"), IP: localIP(), ResourceScope: "unavailable", CPU: CPUStats{Model: model, Count: count, SpeedGHzMin: min, SpeedGHzMax: max}, ProcessUptimeSeconds: 0}
	m.sample()
	go m.loop()
	return m
}

func (m *Monitor) loop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	defer close(m.done)
	for {
		select {
		case <-ticker.C:
			m.sample()
		case <-m.stop:
			return
		}
	}
}
func (m *Monitor) Close() {
	select {
	case <-m.stop:
	default:
		close(m.stop)
	}
	<-m.done
}
func (m *Monitor) Stats() SystemStats { m.mu.RLock(); value := m.current; m.mu.RUnlock(); return value }

func (m *Monitor) sample() {
	now := m.now()
	stats := m.base
	stats.ProcessUptimeSeconds = now.Sub(m.started).Seconds()
	stats.HostUptimeSeconds = uptime(m.read)
	stats.ResourceScope = "unavailable"
	stats.ResourcesAvailable = false
	if m.cgroup != nil {
		stats.ResourceScope = "cgroup-v2"
		cpu, cpuErr := m.cgroup.cpu()
		mem, memErr := m.cgroup.memory()
		stats.ResourcesAvailable = cpuErr == nil && memErr == nil
		if cpuErr == nil {
			stats.CPU.UsageCores, stats.CPU.UsagePercent, stats.CPU.CapacityCores = m.cpuValues(cpu, now)
		}
		if memErr == nil {
			stats.Memory = memoryStats(mem)
		}
	}
	m.mu.Lock()
	m.current = stats
	m.mu.Unlock()
}

func (m *Monitor) cpuValues(readings cpuReadings, now time.Time) (*float64, *float64, *float64) {
	var cores, percent *float64
	if !m.previousAt.IsZero() && readings.usageMicros >= m.previousUsage {
		elapsed := now.Sub(m.previousAt).Microseconds()
		if elapsed > 0 {
			value := float64(readings.usageMicros-m.previousUsage) / float64(elapsed)
			cores = &value
			if readings.capacity != nil && *readings.capacity > 0 {
				pct := value / *readings.capacity * 100
				percent = &pct
			}
		}
	}
	m.previousUsage, m.previousAt = readings.usageMicros, now
	var capacity *float64
	if readings.capacity != nil {
		value := *readings.capacity
		capacity = &value
	}
	return cores, percent, capacity
}

func memoryStats(readings memoryReadings) MemoryStats {
	current, working := readings.current, readings.workingSet
	result := MemoryStats{CurrentBytes: &current, WorkingSetBytes: &working, UsedBytes: &working}
	if readings.limit != nil {
		limit := *readings.limit
		available := limit - working
		if available < 0 {
			available = 0
		}
		pct := float64(working) / float64(limit) * 100
		result.LimitBytes, result.TotalBytes, result.FreeBytes, result.UsagePercent = &limit, &limit, &available, &pct
	}
	return result
}

func readFirstLine(read readFile, path string) string {
	data, _ := read(path)
	return strings.TrimSpace(strings.SplitN(string(data), "\n", 2)[0])
}
func uptime(read readFile) float64 {
	data, err := read("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0
	}
	seconds, _ := strconv.ParseFloat(fields[0], 64)
	return seconds
}
func localIP() string {
	interfaces, _ := net.Interfaces()
	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addresses, _ := iface.Addrs()
		for _, addr := range addresses {
			ip, _, err := net.ParseCIDR(addr.String())
			if err == nil && ip.To4() != nil {
				return ip.String()
			}
		}
	}
	return "127.0.0.1"
}
