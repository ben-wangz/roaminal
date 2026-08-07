package monitor

import (
	"bufio"
	"errors"
	"runtime"
	"strconv"
	"strings"
)

type cpuReadings struct {
	usageMicros uint64
	capacity    *float64
}

func (c *cgroup) cpu() (cpuReadings, error) {
	stat, err := c.file("cpu.stat")
	if err != nil {
		return cpuReadings{}, err
	}
	usage, err := parseCPUUsage(string(stat))
	if err != nil {
		return cpuReadings{}, err
	}
	quota, err := c.file("cpu.max")
	if err != nil {
		return cpuReadings{}, err
	}
	capacity := parseCPUCapacity(string(quota))
	if cpuset, err := c.file("cpuset.cpus.effective"); err == nil {
		if count := parseCPUSetCount(string(cpuset)); count > 0 {
			value := float64(count)
			if capacity == nil || value < *capacity {
				capacity = &value
			}
		}
	}
	return cpuReadings{usageMicros: usage, capacity: capacity}, nil
}

func parseCPUUsage(data string) (uint64, error) {
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "usage_usec" {
			value, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return 0, err
			}
			return value, nil
		}
	}
	return 0, errors.New("cpu usage_usec missing")
}
func parseCPUCapacity(data string) *float64 {
	fields := strings.Fields(data)
	if len(fields) != 2 || fields[0] == "max" {
		return nil
	}
	quota, err1 := strconv.ParseFloat(fields[0], 64)
	period, err2 := strconv.ParseFloat(fields[1], 64)
	if err1 != nil || err2 != nil || quota <= 0 || period <= 0 {
		return nil
	}
	value := quota / period
	return &value
}
func parseCPUSetCount(data string) int {
	total := 0
	for _, part := range strings.Split(strings.TrimSpace(data), ",") {
		if part == "" {
			continue
		}
		bounds := strings.SplitN(part, "-", 2)
		start, err := strconv.Atoi(bounds[0])
		if err != nil {
			return 0
		}
		end := start
		if len(bounds) == 2 {
			end, err = strconv.Atoi(bounds[1])
			if err != nil || end < start {
				return 0
			}
		}
		total += end - start + 1
	}
	return total
}

func cpuInfo(read readFile) (string, int, float64, float64) {
	data, err := read("/proc/cpuinfo")
	if err != nil {
		return runtime.GOARCH, runtime.NumCPU(), 0, 0
	}
	model, count, min, max := runtime.GOARCH, 0, 0.0, 0.0
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), ":")
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
