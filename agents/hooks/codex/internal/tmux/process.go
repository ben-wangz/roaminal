package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

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
