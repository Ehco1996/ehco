package sampler

import (
	"context"
	"time"

	"github.com/Ehco1996/ehco/internal/store"
	"go.uber.org/zap"
)

const defaultSampleInterval = 5 * time.Second

type Collector struct {
	store    *store.Store
	sampler  *NodeSampler
	interval time.Duration
	l        *zap.SugaredLogger
}

func NewCollector(store *store.Store, sampler *NodeSampler) *Collector {
	return &Collector{
		store:    store,
		sampler:  sampler,
		interval: defaultSampleInterval,
		l:        zap.S().Named("sampler.collector"),
	}
}

func (c *Collector) Start(ctx context.Context) {
	c.l.Infof("starting host metrics collector with interval %s", c.interval)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.l.Info("host metrics collector stopped")
			return
		case <-ticker.C:
			c.sampleOnce(ctx)
		}
	}
}

func (c *Collector) sampleOnce(ctx context.Context) {
	nm, err := c.sampler.Sample(ctx)
	if err != nil {
		c.l.Debugf("sample node metrics failed: %v", err)
		return
	}
	sample := &store.Sample{
		Timestamp:                nm.SyncTime.Unix(),
		CpuUsagePercent:          nm.CpuUsagePercent,
		MemoryUsagePercent:       nm.MemoryUsagePercent,
		DiskUsagePercent:         nm.DiskUsagePercent,
		NetworkReceiveBytesRate:  nm.NetworkReceiveBytesRate,
		NetworkTransmitBytesRate: nm.NetworkTransmitBytesRate,
	}
	if err := c.store.AddNodeMetric(ctx, sample); err != nil {
		c.l.Errorf("persist node metric failed: %v", err)
	}
}
