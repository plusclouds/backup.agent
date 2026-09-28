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
package system

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	gopsnet "github.com/shirou/gopsutil/v3/net"

	"github.com/plusclouds/ubuntu-agent/pkg/isoconfig"
)

// Module collects system information and resource metrics from the host VM.
type Module struct {
	iso *isoconfig.ISOMetadata

	// Disk IO delta state — protected by mu.
	mu       sync.Mutex
	prevIO   map[string]disk.IOCountersStat
	prevIOAt time.Time
}

// New creates a new system Module.
func New(iso *isoconfig.ISOMetadata) *Module {
	return &Module{iso: iso}
}

// GetInfo returns static/semi-static information about the host VM.
func (m *Module) GetInfo(ctx context.Context) (*SystemInfo, error) {
	info, err := host.InfoWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting host info: %w", err)
	}

	si := &SystemInfo{
		Hostname:      info.Hostname,
		OS:            fmt.Sprintf("%s %s", info.Platform, info.PlatformVersion),
		KernelVersion: info.KernelVersion,
		Architecture:  info.KernelArch,
		Uptime:        int64(info.Uptime),
		BootTime:      int64(info.BootTime),
	}

	if m.iso != nil {
		si.VMID = m.iso.VMID()
		si.TenantID = m.iso.TenantID()
	}

	return si, nil
}

// GetMetrics returns a point-in-time snapshot conforming to the protocol schema.
func (m *Module) GetMetrics(ctx context.Context) (*SystemMetrics, error) {
	cpuStats, err := m.GetCPU(ctx)
	if err != nil {
		return nil, fmt.Errorf("collecting CPU metrics: %w", err)
	}

	memStats, err := m.GetMemory(ctx)
	if err != nil {
		return nil, fmt.Errorf("collecting memory metrics: %w", err)
	}

	disks, err := m.GetDisk(ctx)
	if err != nil {
		return nil, fmt.Errorf("collecting disk metrics: %w", err)
	}

	network, err := m.GetNetwork(ctx)
	if err != nil {
		return nil, fmt.Errorf("collecting network metrics: %w", err)
	}

	return &SystemMetrics{
		CPU:     *cpuStats,
		Memory:  *memStats,
		Disks:   disks,
		Network: network,
	}, nil
}

// GetCPU returns CPU utilisation, load averages, and per-core usage.
// Overall and per-core usage are sampled over a 500 ms window.
func (m *Module) GetCPU(ctx context.Context) (*CPUStats, error) {
	// Overall usage (percpu=false).
	overall, err := cpu.PercentWithContext(ctx, 500*time.Millisecond, false)
	if err != nil {
		return nil, fmt.Errorf("cpu.Percent (overall): %w", err)
	}
	var usagePct float64
	if len(overall) > 0 {
		usagePct = overall[0]
	}

	coreCount, err := cpu.CountsWithContext(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("cpu.Counts: %w", err)
	}

	// Per-core usage (percpu=true). Uses the same 500 ms already elapsed above,
	// so this call returns immediately (interval=0 reuses the cached sample).
	var cores []CoreStat
	perCore, err := cpu.PercentWithContext(ctx, 0, true)
	if err == nil && len(perCore) > 0 {
		cores = make([]CoreStat, len(perCore))
		for i, pct := range perCore {
			cores[i] = CoreStat{ID: i, UsagePct: pct}
		}
	}
	// If per-core data fails, omit the field (nil slice → omitempty).

	var loadAvg [3]float64
	if avg, err := load.AvgWithContext(ctx); err == nil {
		loadAvg = [3]float64{avg.Load1, avg.Load5, avg.Load15}
	}

	return &CPUStats{
		UsagePct:  usagePct,
		CoreCount: coreCount,
		LoadAvg:   loadAvg,
		Cores:     cores,
	}, nil
}

// GetMemory returns current RAM utilisation.
func (m *Module) GetMemory(ctx context.Context) (*MemoryStats, error) {
	vm, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("mem.VirtualMemory: %w", err)
	}

	return &MemoryStats{
		TotalBytes: vm.Total,
		UsedBytes:  vm.Used,
		UsagePct:   vm.UsedPercent,
	}, nil
}

// pseudoFS is the set of filesystem types excluded from disk telemetry.
var pseudoFS = map[string]bool{
	"tmpfs": true, "devtmpfs": true, "proc": true, "sysfs": true,
	"devpts": true, "cgroup": true, "cgroup2": true, "pstore": true,
	"hugetlbfs": true, "mqueue": true, "debugfs": true, "tracefs": true,
	"securityfs": true, "fusectl": true, "configfs": true, "bpf": true,
	"overlay": true, "squashfs": true, "efivarfs": true,
}

// GetDisk returns usage and I/O stats for real block-device partitions only.
// I/O rates are calculated as a delta from the previous call; the io field is
// omitted on the first call (no baseline).
func (m *Module) GetDisk(ctx context.Context) ([]DiskEntry, error) {
	partitions, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("disk.Partitions: %w", err)
	}

	// Snapshot current I/O counters (keyed by kernel device name, e.g. "xvda2").
	now := time.Now()
	ioCounters, _ := disk.IOCountersWithContext(ctx) // errors are non-fatal

	m.mu.Lock()
	prev := m.prevIO
	prevAt := m.prevIOAt
	m.prevIO = ioCounters
	m.prevIOAt = now
	m.mu.Unlock()

	var elapsed float64
	if !prevAt.IsZero() {
		elapsed = now.Sub(prevAt).Seconds()
	}

	var entries []DiskEntry
	for _, p := range partitions {
		if pseudoFS[p.Fstype] {
			continue
		}
		usage, err := disk.UsageWithContext(ctx, p.Mountpoint)
		if err != nil {
			continue
		}

		entry := DiskEntry{
			Device:     p.Device,
			Mountpoint: p.Mountpoint,
			TotalBytes: usage.Total,
			UsedBytes:  usage.Used,
			UsagePct:   usage.UsedPercent,
		}

		// Attach IO rates when we have a previous baseline and counters.
		if elapsed > 0 && prev != nil && ioCounters != nil {
			// gopsutil keys by kernel name (strip /dev/ prefix).
			devName := strings.TrimPrefix(p.Device, "/dev/")
			if cur, ok := ioCounters[devName]; ok {
				if old, ok := prev[devName]; ok {
					elapsedMs := elapsed * 1000
					utilPct := 0.0
					if elapsedMs > 0 {
						utilPct = float64(cur.IoTime-old.IoTime) / elapsedMs * 100
						if utilPct > 100 {
							utilPct = 100
						}
					}
					entry.IO = &DiskIO{
						ReadBytesPerS:  float64(cur.ReadBytes-old.ReadBytes) / elapsed,
						WriteBytesPerS: float64(cur.WriteBytes-old.WriteBytes) / elapsed,
						ReadIOPS:       float64(cur.ReadCount-old.ReadCount) / elapsed,
						WriteIOPS:      float64(cur.WriteCount-old.WriteCount) / elapsed,
						UtilPct:        utilPct,
					}
				}
			}
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// isVirtualInterface reports whether the interface name should be excluded
// from telemetry (loopback, Docker bridges, veth pairs, libvirt bridges).
func isVirtualInterface(name string) bool {
	return name == "lo" ||
		strings.HasPrefix(name, "docker") ||
		strings.HasPrefix(name, "veth") ||
		strings.HasPrefix(name, "br-") ||
		strings.HasPrefix(name, "virbr") ||
		strings.HasPrefix(name, "vlan")
}

// GetNetwork returns I/O counters for physical/relevant interfaces only.
func (m *Module) GetNetwork(ctx context.Context) ([]NetworkEntry, error) {
	counters, err := gopsnet.IOCountersWithContext(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("net.IOCounters: %w", err)
	}

	ifaces, err := gopsnet.InterfacesWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("net.Interfaces: %w", err)
	}

	ifaceMap := make(map[string]gopsnet.InterfaceStat, len(ifaces))
	for _, iface := range ifaces {
		ifaceMap[iface.Name] = iface
	}

	var entries []NetworkEntry
	for _, c := range counters {
		if isVirtualInterface(c.Name) {
			continue
		}

		entry := NetworkEntry{
			Interface: c.Name,
			BytesSent: c.BytesSent,
			BytesRecv: c.BytesRecv,
		}

		if netIface, ok := ifaceMap[c.Name]; ok {
			for _, flag := range netIface.Flags {
				if flag == "up" {
					entry.IsUp = true
					break
				}
			}
		}

		entries = append(entries, entry)
	}

	return entries, nil
}
