package monitor

import "time"

type RemoteMonitorSnapshot struct {
	Status     string               `json:"status"`
	SampledAt  *time.Time           `json:"sampledAt"`
	AgeMs      *int64               `json:"ageMs"`
	Metrics    RemoteMonitorMetrics `json:"metrics"`
	ProbeRttMs *int64               `json:"probeRttMs"`
}

type RemoteMonitorMetrics struct {
	CPU    RemoteCPUMetric    `json:"cpu"`
	Memory RemoteMemoryMetric `json:"memory"`
	Uptime RemoteUptimeMetric `json:"uptime"`
	Load   RemoteLoadMetric   `json:"load"`
	Disk   RemoteDiskMetric   `json:"disk"`
}

type RemoteCPUMetric struct {
	Status        string   `json:"status"`
	Scope         string   `json:"scope"`
	Percent       *float64 `json:"percent"`
	UsageCores    *float64 `json:"usageCores"`
	CapacityCores *float64 `json:"capacityCores"`
}

type RemoteMemoryMetric struct {
	Status          string   `json:"status"`
	Scope           string   `json:"scope"`
	WorkingSetBytes *int64   `json:"workingSetBytes"`
	CurrentBytes    *int64   `json:"currentBytes"`
	LimitBytes      *int64   `json:"limitBytes"`
	Percent         *float64 `json:"percent"`
}

type RemoteUptimeMetric struct {
	Status  string   `json:"status"`
	Scope   string   `json:"scope"`
	Seconds *float64 `json:"seconds"`
}

type RemoteLoadMetric struct {
	Status  string   `json:"status"`
	Scope   string   `json:"scope"`
	One     *float64 `json:"one"`
	Five    *float64 `json:"five"`
	Fifteen *float64 `json:"fifteen"`
}

type RemoteDiskMetric struct {
	Status         string   `json:"status"`
	Scope          string   `json:"scope"`
	Mount          string   `json:"mount"`
	TotalBytes     *int64   `json:"totalBytes"`
	UsedBytes      *int64   `json:"usedBytes"`
	AvailableBytes *int64   `json:"availableBytes"`
	Percent        *float64 `json:"percent"`
}

type remoteMonitorState struct {
	snapshot *RemoteMonitorSnapshot
	baseline *remoteCPUBaseline
	lastGood time.Time
	failures int
	inflight chan struct{}
}

type remoteCPUBaseline struct {
	scope     string
	usageNS   uint64
	hostTotal uint64
	hostIdle  uint64
	sampledAt time.Time
}

type remoteRawSample struct {
	scope                    string
	cpuUsageNS               *uint64
	cpuCapacityMilli         *uint64
	hostCPUTotalTicks        *uint64
	hostCPUIdleTicks         *uint64
	memoryCurrentBytes       *uint64
	memoryInactiveBytes      *uint64
	memoryLimitBytes         *uint64
	hostMemoryTotalBytes     *uint64
	hostMemoryAvailableBytes *uint64
	pid1StartTicks           *uint64
	clockTicks               *uint64
	systemUptimeSeconds      *float64
	loadOne                  *float64
	loadFive                 *float64
	loadFifteen              *float64
	rootfsTotalKiB           *uint64
	rootfsUsedKiB            *uint64
	rootfsAvailableKiB       *uint64
	rootfsPercent            *float64
}
