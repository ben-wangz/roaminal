package monitor

import (
	"bufio"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CPUStats struct {
	Model        string  `json:"model"`
	Count        int     `json:"count"`
	SpeedGHzMin  float64 `json:"speedGHzMin"`
	SpeedGHzMax  float64 `json:"speedGHzMax"`
	UsagePercent float64 `json:"usagePercent"`
}

type MemoryStats struct {
	TotalBytes int64 `json:"totalBytes"`
	UsedBytes  int64 `json:"usedBytes"`
	FreeBytes  int64 `json:"freeBytes"`
}

type SystemStats struct {
	Hostname             string      `json:"hostname"`
	Kernel               string      `json:"kernel"`
	IP                   string      `json:"ip"`
	CPU                  CPUStats    `json:"cpu"`
	Memory               MemoryStats `json:"memory"`
	HostUptimeSeconds    float64     `json:"hostUptimeSeconds"`
	ProcessUptimeSeconds float64     `json:"processUptimeSeconds"`
}

type Monitor struct {
	started   time.Time
	mu        sync.Mutex
	lastCPU   time.Time
	lastIdle  uint64
	lastTotal uint64
}

func New() *Monitor { return &Monitor{started: time.Now()} }

func (m *Monitor) Stats() SystemStats {
	host, _ := os.Hostname()
	kernel := readFirstLine("/proc/sys/kernel/osrelease")
	return SystemStats{Hostname: host, Kernel: kernel, IP: localIP(), CPU: m.cpu(), Memory: memory(), HostUptimeSeconds: uptime(), ProcessUptimeSeconds: time.Since(m.started).Seconds()}
}

func (m *Monitor) cpu() CPUStats {
	model, count, min, max := cpuInfo()
	usage := 0.0
	if data, err := os.ReadFile("/proc/stat"); err == nil {
		line := strings.Split(string(data), "\n")[0]
		fields := strings.Fields(line)
		if len(fields) >= 5 {
			var total, idle uint64
			for i := 1; i < len(fields); i++ {
				value, _ := strconv.ParseUint(fields[i], 10, 64)
				total += value
				if i == 4 {
					idle = value
				}
			}
			m.mu.Lock()
			if m.lastTotal != 0 && total > m.lastTotal {
				usage = float64((total-m.lastTotal)-(idle-m.lastIdle)) / float64(total-m.lastTotal) * 100
			}
			m.lastTotal, m.lastIdle, m.lastCPU = total, idle, time.Now()
			m.mu.Unlock()
		}
	}
	return CPUStats{Model: model, Count: count, SpeedGHzMin: min, SpeedGHzMax: max, UsagePercent: usage}
}

func cpuInfo() (string, int, float64, float64) {
	file, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return runtime.GOARCH, runtime.NumCPU(), 0, 0
	}
	defer file.Close()
	model, count := runtime.GOARCH, 0
	min, max := 0.0, 0.0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "model name":
			if model == runtime.GOARCH {
				model = value
			}
		case "processor":
			count++
		case "cpu MHz":
			mhz, _ := strconv.ParseFloat(value, 64)
			ghz := mhz / 1000
			if min == 0 || ghz < min {
				min = ghz
			}
			if ghz > max {
				max = ghz
			}
		}
	}
	if count == 0 {
		count = runtime.NumCPU()
	}
	return model, count, min, max
}

func memory() MemoryStats {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return MemoryStats{}
	}
	values := map[string]int64{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			value, _ := strconv.ParseInt(fields[1], 10, 64)
			values[strings.TrimSuffix(fields[0], ":")] = value * 1024
		}
	}
	total, free := values["MemTotal"], values["MemAvailable"]
	return MemoryStats{TotalBytes: total, FreeBytes: free, UsedBytes: total - free}
}

func uptime() float64 {
	value, err := strconv.ParseFloat(strings.TrimSpace(readFirstLine("/proc/uptime")), 64)
	if err != nil {
		return 0
	}
	return value
}
func readFirstLine(path string) string {
	data, _ := os.ReadFile(path)
	return strings.TrimSpace(strings.SplitN(string(data), "\n", 2)[0])
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
