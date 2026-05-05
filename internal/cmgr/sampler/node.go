package sampler

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	psnet "github.com/shirou/gopsutil/v4/net"
	"go.uber.org/zap"
)

// NodeSampler reads host stats directly via gopsutil. It replaces the
// node_exporter → /metrics → expfmt detour the project used to take when
// the source and sink lived in the same Go process.
//
// Sample is not goroutine-safe; the caller (cmgr's tick loop) is expected
// to invoke it serially.
type NodeSampler struct {
	last *NodeMetrics
	l    *zap.SugaredLogger
}

func NewNodeSampler() *NodeSampler {
	return &NodeSampler{l: zap.S().Named("sampler.node")}
}

func (s *NodeSampler) Sample(ctx context.Context) (*NodeMetrics, error) {
	now := time.Now()
	nm := &NodeMetrics{SyncTime: now}

	if pct, err := cpu.PercentWithContext(ctx, 0, false); err == nil && len(pct) > 0 {
		nm.CpuUsagePercent = round2(pct[0])
	} else if err != nil {
		s.l.Debugf("cpu.Percent: %v", err)
	}
	nm.CpuCoreCount = runtime.NumCPU()

	if avg, err := load.AvgWithContext(ctx); err == nil {
		nm.CpuLoadInfo = fmt.Sprintf("%.2f|%.2f|%.2f", avg.Load1, avg.Load5, avg.Load15)
	} else {
		s.l.Debugf("load.Avg: %v", err)
	}

	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		nm.MemoryTotalBytes = int64(vm.Total)
		nm.MemoryUsageBytes = int64(vm.Used)
		nm.MemoryUsagePercent = round2(vm.UsedPercent)
	} else {
		s.l.Debugf("mem.VirtualMemory: %v", err)
	}

	if du, err := disk.UsageWithContext(ctx, "/"); err == nil {
		nm.DiskTotalBytes = int64(du.Total)
		nm.DiskUsageBytes = int64(du.Used)
		nm.DiskUsagePercent = round2(du.UsedPercent)
	} else {
		s.l.Debugf("disk.Usage(/): %v", err)
	}

	if io, err := psnet.IOCountersWithContext(ctx, false); err == nil && len(io) > 0 {
		nm.NetworkReceiveBytesTotal = int64(io[0].BytesRecv)
		nm.NetworkTransmitBytesTotal = int64(io[0].BytesSent)
	} else if err != nil {
		s.l.Debugf("net.IOCounters: %v", err)
	}

	if s.last != nil {
		dur := now.Sub(s.last.SyncTime).Seconds()
		if dur > 0.1 {
			nm.NetworkReceiveBytesRate = math.Round(math.Max(0,
				float64(nm.NetworkReceiveBytesTotal-s.last.NetworkReceiveBytesTotal)/dur))
			nm.NetworkTransmitBytesRate = math.Round(math.Max(0,
				float64(nm.NetworkTransmitBytesTotal-s.last.NetworkTransmitBytesTotal)/dur))
		}
	}
	s.last = nm
	return nm, nil
}

func round2(f float64) float64 {
	return math.Round(f*100) / 100
}
