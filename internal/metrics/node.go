package metrics

import (
	"context"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"go.uber.org/zap"
)

const nodeCollectInterval = 10 * time.Second

func startNodeCollector(ctx context.Context, l *zap.SugaredLogger) {
	ticker := time.NewTicker(nodeCollectInterval)
	defer ticker.Stop()

	var prevRx, prevTx uint64
	var prevTime time.Time

	collect := func() {
		var cpuPct, memPct, dskPct, netIn, netOut float64

		if pcts, err := cpu.PercentWithContext(ctx, 0, false); err == nil && len(pcts) > 0 {
			cpuPct = pcts[0]
		} else if err != nil {
			l.Debugf("cpu percent: %v", err)
		}
		if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
			memPct = vm.UsedPercent
		} else {
			l.Debugf("vm: %v", err)
		}
		if du, err := disk.UsageWithContext(ctx, "/"); err == nil {
			dskPct = du.UsedPercent
		} else {
			l.Debugf("disk usage: %v", err)
		}

		if io, err := net.IOCountersWithContext(ctx, false); err == nil && len(io) > 0 {
			rx := io[0].BytesRecv
			tx := io[0].BytesSent
			now := time.Now()
			if !prevTime.IsZero() {
				dt := now.Sub(prevTime).Seconds()
				if dt > 0 {
					if rx >= prevRx {
						netIn = float64(rx-prevRx) / dt
					}
					if tx >= prevTx {
						netOut = float64(tx-prevTx) / dt
					}
				}
			}
			prevRx, prevTx, prevTime = rx, tx, now
		} else if err != nil {
			l.Debugf("net io: %v", err)
		}

		globalStore.setNode(cpuPct, memPct, dskPct, netIn, netOut)
	}

	collect()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			collect()
		}
	}
}
