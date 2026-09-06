package ms

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ehco1996/ehco/internal/cmgr/sampler"
)

func newTestStore(t *testing.T) *MetricsStore {
	t.Helper()
	ms, err := NewMetricsStore(filepath.Join(t.TempDir(), "metrics_ts"))
	if err != nil {
		t.Fatalf("NewMetricsStore: %v", err)
	}
	t.Cleanup(func() { _ = ms.Close() })
	return ms
}

func TestHealth_EmptyStore(t *testing.T) {
	ms := newTestStore(t)
	h, err := ms.Health(context.Background())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if h.NodeMetricsRows != 0 {
		t.Fatalf("expected empty store, got node=%d", h.NodeMetricsRows)
	}
	if _, ok := h.Stats["query_node"]; !ok {
		t.Fatalf("stats map missing query_node key")
	}
}

func TestHealth_TracksWritesAndQueries(t *testing.T) {
	ms := newTestStore(t)
	ctx := context.Background()

	now := time.Now()
	if err := ms.AddNodeMetric(ctx, &sampler.NodeMetrics{
		SyncTime:                 now,
		CpuUsagePercent:          1,
		MemoryUsagePercent:       2,
		DiskUsagePercent:         3,
		NetworkReceiveBytesRate:  4,
		NetworkTransmitBytesRate: 5,
	}); err != nil {
		t.Fatalf("AddNodeMetric: %v", err)
	}

	resp, err := ms.QueryNodeMetric(ctx, &QueryNodeMetricsReq{
		StartTimestamp: 0,
		EndTimestamp:   now.Unix() + 1,
		Num:            10,
	})
	if err != nil {
		t.Fatalf("QueryNodeMetric: %v", err)
	}
	if resp.TOTAL != 1 {
		t.Fatalf("expected 1 record, got %d", resp.TOTAL)
	}
	if resp.Data[0].CPUUsage != 1 {
		t.Fatalf("expected cpu_usage=1, got %v", resp.Data[0].CPUUsage)
	}

	h, err := ms.Health(ctx)
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if h.NodeMetricsRows != 1 {
		t.Fatalf("expected 1 node row, got %d", h.NodeMetricsRows)
	}
	if h.Stats["add_node"].Count != 1 {
		t.Fatalf("expected add_node count=1, got %d", h.Stats["add_node"].Count)
	}
	if h.Stats["query_node"].Count != 1 {
		t.Fatalf("expected query_node count=1, got %d", h.Stats["query_node"].Count)
	}
	if h.Stats["add_node"].LastMs < 0 {
		t.Fatalf("expected non-negative last_ms for add_node, got %v", h.Stats["add_node"].LastMs)
	}
}

func TestTruncate_RequiresExactConfirm(t *testing.T) {
	ms := newTestStore(t)
	ctx := context.Background()
	if err := ms.AddNodeMetric(ctx, &sampler.NodeMetrics{SyncTime: time.Now()}); err != nil {
		t.Fatalf("AddNodeMetric: %v", err)
	}

	for _, bad := range []string{"", "yes", "true", "YES I AM SURE"} {
		if _, err := ms.Truncate(ctx, bad); err == nil {
			t.Fatalf("expected Truncate(%q) to fail", bad)
		}
	}
	if _, err := ms.Truncate(ctx, truncateConfirm); err != nil {
		t.Fatalf("Truncate with valid confirm: %v", err)
	}
	h, _ := ms.Health(ctx)
	if h.NodeMetricsRows != 0 {
		t.Fatalf("expected empty after truncate, got %d", h.NodeMetricsRows)
	}
}

func TestResetStats_ClearsCounters(t *testing.T) {
	ms := newTestStore(t)
	ctx := context.Background()
	_ = ms.AddNodeMetric(ctx, &sampler.NodeMetrics{SyncTime: time.Now()})
	if h, _ := ms.Health(ctx); h.Stats["add_node"].Count != 1 {
		t.Fatalf("setup: expected add_node count=1")
	}
	ms.ResetStats()
	h, _ := ms.Health(ctx)
	if h.Stats["add_node"].Count != 0 {
		t.Fatalf("expected count=0 after reset, got %d", h.Stats["add_node"].Count)
	}
}

func TestDownsample_StepBuckets(t *testing.T) {
	ms := newTestStore(t)
	ctx := context.Background()

	base := time.Unix(1700000000, 0)
	// Add 10 points spaced 5s apart (spanning 0s to 45s)
	for i := 0; i < 10; i++ {
		ts := base.Add(time.Duration(i*5) * time.Second)
		err := ms.AddNodeMetric(ctx, &sampler.NodeMetrics{
			SyncTime:        ts,
			CpuUsagePercent: float64(10 + i),
		})
		if err != nil {
			t.Fatalf("AddNodeMetric: %v", err)
		}
	}

	// Step = 20s. Points in [0s, 15s] fall into bucket 0, points in [20s, 35s] fall into bucket 20, points in [40s, 45s] into bucket 40.
	resp, err := ms.QueryNodeMetric(ctx, &QueryNodeMetricsReq{
		StartTimestamp: base.Unix(),
		EndTimestamp:   base.Add(60 * time.Second).Unix(),
		Step:           20,
	})
	if err != nil {
		t.Fatalf("QueryNodeMetric: %v", err)
	}
	if len(resp.Data) != 3 {
		t.Fatalf("expected 3 step buckets, got %d", len(resp.Data))
	}
	// Should be sorted DESC
	if resp.Data[0].Timestamp < resp.Data[1].Timestamp {
		t.Fatalf("expected DESC order, got %d < %d", resp.Data[0].Timestamp, resp.Data[1].Timestamp)
	}
}
