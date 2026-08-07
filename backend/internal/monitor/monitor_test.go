package monitor

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func fixtureReader(values map[string]string) readFile {
	return func(path string) ([]byte, error) {
		value, ok := values[path]
		if !ok {
			return nil, errors.New("missing fixture: " + path)
		}
		return []byte(value), nil
	}
}

func TestCgroupDiscoverySupportsNamespaceRootAndNestedMount(t *testing.T) {
	for _, test := range []struct{ name, cgroup, mount, want string }{
		{name: "namespace root", cgroup: "0::/\n", mount: "12 1 0:42 / /sys/fs/cgroup rw - cgroup2 cgroup rw\n", want: "/sys/fs/cgroup/memory.current"},
		{name: "nested", cgroup: "0::/kubepods.slice/pod.scope\n", mount: "12 1 0:42 /kubepods.slice /sys/fs/cgroup rw - cgroup2 cgroup rw\n", want: "/sys/fs/cgroup/pod.scope/memory.current"},
	} {
		t.Run(test.name, func(t *testing.T) {
			values := map[string]string{"/proc/self/cgroup": test.cgroup, "/proc/self/mountinfo": test.mount}
			cg, err := discoverCgroup(fixtureReader(values))
			if err != nil {
				t.Fatal(err)
			}
			if got := filepath.Join(cg.mountpoint, cg.relative, "memory.current"); got != test.want {
				t.Fatalf("path=%q want %q", got, test.want)
			}
		})
	}
}

func TestCgroupParsers(t *testing.T) {
	if got := parseCPUCapacity("200000 100000\n"); got == nil || *got != 2 {
		t.Fatalf("quota capacity=%v", got)
	}
	if got := parseCPUCapacity("max 100000\n"); got != nil {
		t.Fatalf("unlimited quota=%v", *got)
	}
	if got := parseCPUSetCount("0-1,4,6-7\n"); got != 5 {
		t.Fatalf("cpuset count=%d", got)
	}
	if got := parseMemoryLimit("max\n"); got != nil {
		t.Fatal("expected unlimited memory")
	}
	if got := parseMemoryStat("inactive_file 80\n", "inactive_file"); got != 80 {
		t.Fatalf("inactive=%d", got)
	}
}

func TestMonitorUsesCgroupDeltaAndWorkingSet(t *testing.T) {
	values := map[string]string{
		"/proc/self/cgroup": "0::/\n", "/proc/self/mountinfo": "12 1 0:42 / /sys/fs/cgroup rw - cgroup2 cgroup rw\n",
		"/proc/cpuinfo": "processor: 0\n", "/etc/hostname": "pod\n", "/proc/sys/kernel/osrelease": "kernel\n", "/proc/uptime": "100.0 0.0\n",
		"/sys/fs/cgroup/cpu.stat": "usage_usec 100000\n", "/sys/fs/cgroup/cpu.max": "200000 100000\n", "/sys/fs/cgroup/cpuset.cpus.effective": "0-1\n",
		"/sys/fs/cgroup/memory.current": "1000\n", "/sys/fs/cgroup/memory.stat": "inactive_file 1200\n", "/sys/fs/cgroup/memory.max": "2000\n",
	}
	now := time.Unix(100, 0)
	read := fixtureReader(values)
	monitor := NewWithReader(read, func() time.Time { return now })
	defer monitor.Close()
	values["/sys/fs/cgroup/cpu.stat"] = "usage_usec 200000\n"
	now = now.Add(time.Second)
	monitor.sample()
	stats := monitor.Stats()
	if !stats.ResourcesAvailable || stats.ResourceScope != "cgroup-v2" {
		t.Fatalf("resources=%+v", stats)
	}
	if stats.CPU.UsageCores == nil || *stats.CPU.UsageCores != 0.1 {
		t.Fatalf("cpu=%+v", stats.CPU.UsageCores)
	}
	if stats.CPU.UsagePercent == nil || *stats.CPU.UsagePercent != 5 {
		t.Fatalf("percent=%+v", stats.CPU.UsagePercent)
	}
	if stats.Memory.WorkingSetBytes == nil || *stats.Memory.WorkingSetBytes != 0 {
		t.Fatalf("working set=%+v", stats.Memory.WorkingSetBytes)
	}
}
