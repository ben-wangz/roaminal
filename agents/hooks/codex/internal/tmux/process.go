package tmux

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func ownerIdentity() string {
	pid, start := codexProcessIdentity()
	return fmt.Sprintf("%d|%s", pid, start)
}

func codexProcessIdentity() (int, string) {
	pid := os.Getppid()
	for depth := 0; depth < 12 && pid > 1; depth++ {
		if strings.Contains(strings.ToLower(filepath.Base(processName(pid))), "codex") {
			return pid, processStart(pid)
		}
		parent := processParent(pid)
		if parent < 1 || parent == pid {
			break
		}
		pid = parent
	}
	pid = os.Getppid()
	return pid, processStart(pid)
}

// AgentProcessID is an opaque diagnostic identity for the Codex ancestor. It
// intentionally excludes the raw PID from the event payload.
func AgentProcessID(codexSessionID string) string {
	if codexSessionID == "" {
		return ""
	}
	pid, start := codexProcessIdentity()
	if pid < 1 {
		return ""
	}
	digest := sha256.Sum256([]byte(strconv.Itoa(pid) + start + codexSessionID))
	return base64.RawURLEncoding.EncodeToString(digest[:16])
}

func ownerAlive(value string) bool {
	parts := strings.Split(value, "|")
	if len(parts) < 3 {
		return false
	}
	pid, err := strconv.Atoi(parts[len(parts)-2])
	if err != nil || !processAlive(pid) {
		return false
	}
	start := parts[len(parts)-1]
	return start == "" || start == processStart(pid)
}

func processName(pid int) string {
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
		if err == nil {
			return strings.TrimSpace(string(data))
		}
	}
	output, err := exec.Command("ps", "-o", "comm=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func processParent(pid int) int {
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if err != nil {
			return 0
		}
		closeName := strings.LastIndex(string(data), ")")
		if closeName < 0 {
			return 0
		}
		fields := strings.Fields(string(data)[closeName+1:])
		if len(fields) < 2 {
			return 0
		}
		parent, _ := strconv.Atoi(fields[1])
		return parent
	}
	output, err := exec.Command("ps", "-o", "ppid=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0
	}
	parent, _ := strconv.Atoi(strings.TrimSpace(string(output)))
	return parent
}

func processAncestors(pid int) map[int]struct{} {
	result := map[int]struct{}{}
	for depth := 0; depth < 32 && pid > 1; depth++ {
		if _, exists := result[pid]; exists {
			break
		}
		result[pid] = struct{}{}
		parent := processParent(pid)
		if parent < 1 || parent == pid {
			break
		}
		pid = parent
	}
	return result
}

func processStart(pid int) string {
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if err != nil {
			return ""
		}
		closeName := strings.LastIndex(string(data), ")")
		if closeName < 0 {
			return ""
		}
		fields := strings.Fields(string(data)[closeName+1:])
		if len(fields) < 20 {
			return ""
		}
		return fields[19]
	}
	output, err := exec.Command("ps", "-o", "lstart=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func ProcessStart(pid int) string { return processStart(pid) }

func ProcessAlive(pid int) bool { return processAlive(pid) }
