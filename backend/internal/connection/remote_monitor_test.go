package connection

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestParseRemoteCollectorAllowsBannerAndRejectsDuplicate(t *testing.T) {
	nonce := "0123456789abcdef"
	output := "motd\nROAMINAL_MONITOR_V1_BEGIN_" + nonce + "\n" +
		"scope=cgroup-v2\n" +
		"cpu_usage_ns=1000\n" +
		"memory_current_bytes=200\n" +
		"memory_inactive_file_bytes=20\n" +
		"load_1=0.10\nload_5=0.20\nload_15=0.30\n" +
		"ROAMINAL_MONITOR_V1_END_" + nonce + "\ntrailer\n"
	raw, err := parseRemoteCollector([]byte(output), nonce)
	if err != nil || raw.scope != "cgroup-v2" || raw.cpuUsageNS == nil || *raw.cpuUsageNS != 1000 {
		t.Fatalf("parse = %#v, %v", raw, err)
	}
	if _, err := parseRemoteCollector([]byte(strings.Replace(output, "cpu_usage_ns=1000", "cpu_usage_ns=1000\ncpu_usage_ns=2000", 1)), nonce); err == nil {
		t.Fatal("duplicate field accepted")
	}
}

func TestRemoteCollectorScriptProducesVersionedOutput(t *testing.T) {
	nonce := "0123456789abcdef"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "sh", "-s", "--", nonce)
	command.Stdin = strings.NewReader(remoteCollectorScript)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("collector: %v", err)
	}
	if _, err := parseRemoteCollector(output, nonce); err != nil {
		t.Fatalf("collector output rejected: %v\n%s", err, output)
	}
}

func TestRemoteMetricCalculations(t *testing.T) {
	first := uint64(1_000_000_000)
	second := uint64(1_500_000_000)
	previous := &remoteCPUBaseline{scope: "cgroup-v2", usageNS: first, sampledAt: time.Unix(0, 0)}
	metric, next := cpuMetric(previous, remoteRawSample{scope: "cgroup-v2", cpuUsageNS: &second}, time.Unix(1, 0))
	if metric.Status != "available" || metric.UsageCores == nil || *metric.UsageCores != .5 || next == nil {
		t.Fatalf("cpu metric = %#v", metric)
	}
	raw := remoteRawSample{scope: "cgroup-v2", memoryCurrentBytes: pointerUint64(100), memoryInactiveBytes: pointerUint64(120), memoryLimitBytes: pointerUint64(200)}
	memory := memoryMetric(raw)
	if memory.Status != "available" || memory.WorkingSetBytes == nil || *memory.WorkingSetBytes != 0 {
		t.Fatalf("memory metric = %#v", memory)
	}
}

func TestRemoteProbePoolQueuesInsteadOfReturningUnavailable(t *testing.T) {
	m := &Manager{remoteSem: make(chan struct{}, 1)}
	if !m.acquireRemoteProbe(context.Background()) {
		t.Fatal("first probe did not acquire the pool")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	acquired := make(chan bool, 1)
	go func() { acquired <- m.acquireRemoteProbe(ctx) }()

	select {
	case <-acquired:
		t.Fatal("second probe bypassed the full pool")
	case <-time.After(25 * time.Millisecond):
	}

	m.releaseRemoteProbe()
	select {
	case ok := <-acquired:
		if !ok {
			t.Fatal("queued probe did not acquire after capacity was released")
		}
	case <-time.After(time.Second):
		t.Fatal("queued probe remained blocked after capacity was released")
	}
	m.releaseRemoteProbe()
}

func TestRemoteProbePoolHonorsContextWhileWaiting(t *testing.T) {
	m := &Manager{remoteSem: make(chan struct{}, 1)}
	if !m.acquireRemoteProbe(context.Background()) {
		t.Fatal("first probe did not acquire the pool")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if m.acquireRemoteProbe(ctx) {
		t.Fatal("probe acquired a slot after its context expired")
	}
	m.releaseRemoteProbe()
}

func pointerUint64(value uint64) *uint64 { return &value }
