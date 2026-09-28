// MIT License
//
// Copyright (c) 2026 PlusClouds
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.
// Package system provides types and logic for collecting system information
// and resource metrics from the host VM using gopsutil.
package system

// SystemInfo holds static/semi-static information about the host VM.
type SystemInfo struct {
	Hostname      string `json:"hostname"`
	OS            string `json:"os"`
	KernelVersion string `json:"kernel_version"`
	Architecture  string `json:"architecture"`
	Uptime        int64  `json:"uptime"`
	BootTime      int64  `json:"boot_time"`
	VMID          string `json:"vm_id,omitempty"`
	TenantID      string `json:"tenant_id,omitempty"`
}

// CoreStat holds usage for a single logical CPU core.
type CoreStat struct {
	ID       int     `json:"id"`
	UsagePct float64 `json:"usage_pct"`
}

// CPUStats contains current CPU utilisation and load average data.
type CPUStats struct {
	// UsagePct is overall CPU usage across all cores (0–100).
	UsagePct float64 `json:"usage_pct"`
	// CoreCount is the number of logical CPU cores.
	CoreCount int `json:"core_count"`
	// LoadAvg is [1-min, 5-min, 15-min] load averages. Always [0,0,0] on Windows.
	LoadAvg [3]float64 `json:"load_avg"`
	// Cores is per-core usage. Omitted when per-core data is unavailable.
	Cores []CoreStat `json:"cores,omitempty"`
}

// MemoryStats contains current RAM utilisation.
type MemoryStats struct {
	TotalBytes uint64  `json:"total_bytes"`
	UsedBytes  uint64  `json:"used_bytes"`
	UsagePct   float64 `json:"usage_pct"`
}

// DiskIO holds I/O throughput and utilisation for a single block device.
// Rates are calculated as a delta between two consecutive telemetry snapshots.
// The field is omitted on the first snapshot (no previous baseline).
type DiskIO struct {
	ReadBytesPerS  float64 `json:"read_bytes_per_s"`
	WriteBytesPerS float64 `json:"write_bytes_per_s"`
	ReadIOPS       float64 `json:"read_iops"`
	WriteIOPS      float64 `json:"write_iops"`
	// UtilPct is the percentage of time the device was busy (0–100).
	UtilPct float64 `json:"util_pct"`
}

// DiskEntry holds usage for a single real block-device partition.
// Pseudo-filesystems (tmpfs, devtmpfs, proc, etc.) are excluded.
type DiskEntry struct {
	Device     string  `json:"device"`
	Mountpoint string  `json:"mountpoint"`
	TotalBytes uint64  `json:"total_bytes"`
	UsedBytes  uint64  `json:"used_bytes"`
	UsagePct   float64 `json:"usage_pct"`
	// IO is nil on the first snapshot and whenever I/O counters are unavailable.
	IO *DiskIO `json:"io,omitempty"`
}

// NetworkEntry holds I/O counters for a single physical network interface.
// Loopback (lo), docker*, veth*, and bridge interfaces are excluded.
type NetworkEntry struct {
	Interface string `json:"interface"`
	BytesSent uint64 `json:"bytes_sent"`
	BytesRecv uint64 `json:"bytes_recv"`
	IsUp      bool   `json:"is_up"`
}

// SystemMetrics is a point-in-time snapshot conforming to the platform
// telemetry payload schema defined in docs/agents/protocol.md.
type SystemMetrics struct {
	CPU     CPUStats       `json:"cpu"`
	Memory  MemoryStats    `json:"memory"`
	Disks   []DiskEntry    `json:"disks"`
	Network []NetworkEntry `json:"network"`
}
