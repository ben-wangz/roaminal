package monitor

import (
	"errors"
	"strconv"
	"strings"
)

type memoryReadings struct {
	current, workingSet int64
	limit               *int64
}

func (c *cgroup) memory() (memoryReadings, error) {
	currentData, err := c.file("memory.current")
	if err != nil {
		return memoryReadings{}, err
	}
	current, err := parseBytes(string(currentData))
	if err != nil {
		return memoryReadings{}, err
	}
	statData, err := c.file("memory.stat")
	if err != nil {
		return memoryReadings{}, err
	}
	inactive := parseMemoryStat(string(statData), "inactive_file")
	workingSet := current - inactive
	if workingSet < 0 {
		workingSet = 0
	}
	limitData, err := c.file("memory.max")
	if err != nil {
		return memoryReadings{}, err
	}
	limit := parseMemoryLimit(string(limitData))
	return memoryReadings{current: current, workingSet: workingSet, limit: limit}, nil
}

func parseBytes(data string) (int64, error) {
	value, err := strconv.ParseInt(strings.TrimSpace(data), 10, 64)
	if err != nil || value < 0 {
		if err == nil {
			err = errors.New("negative byte value")
		}
		return 0, err
	}
	return value, nil
}
func parseMemoryLimit(data string) *int64 {
	value := strings.TrimSpace(data)
	if value == "max" {
		return nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return nil
	}
	return &parsed
}
func parseMemoryStat(data, key string) int64 {
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == key {
			value, _ := strconv.ParseInt(fields[1], 10, 64)
			if value > 0 {
				return value
			}
			return 0
		}
	}
	return 0
}
