package monitor

import "time"

type CPUStats struct {
	Model         string   `json:"model"`
	Count         int      `json:"count"`
	SpeedGHzMin   float64  `json:"speedGHzMin"`
	SpeedGHzMax   float64  `json:"speedGHzMax"`
	UsagePercent  *float64 `json:"usagePercent"`
	UsageCores    *float64 `json:"usageCores"`
	CapacityCores *float64 `json:"capacityCores"`
}

type MemoryStats struct {
	TotalBytes      *int64   `json:"totalBytes"`
	UsedBytes       *int64   `json:"usedBytes"`
	FreeBytes       *int64   `json:"freeBytes"`
	CurrentBytes    *int64   `json:"currentBytes"`
	WorkingSetBytes *int64   `json:"workingSetBytes"`
	LimitBytes      *int64   `json:"limitBytes"`
	UsagePercent    *float64 `json:"usagePercent"`
}

type SystemStats struct {
	Hostname             string      `json:"hostname"`
	Kernel               string      `json:"kernel"`
	IP                   string      `json:"ip"`
	ResourceScope        string      `json:"resourceScope"`
	ResourcesAvailable   bool        `json:"resourcesAvailable"`
	CPU                  CPUStats    `json:"cpu"`
	Memory               MemoryStats `json:"memory"`
	HostUptimeSeconds    float64     `json:"hostUptimeSeconds"`
	ProcessUptimeSeconds float64     `json:"processUptimeSeconds"`
}

type readFile func(string) ([]byte, error)
type clock func() time.Time

func pointer[T any](value T) *T { return &value }
