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
	ms, err := NewMetricsStore(filepath.Join(t.TempDir(), "metrics.db"))
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
	if h.NodeMetricsRows != 0 || h.RuleMetricsRows != 0 {
		t.Fatalf("expected empty store, got node=%d rule=%d", h.NodeMetricsRows, h.RuleMetricsRows)
	}
	if h.PageSize == 0 {
		t.Fatalf("page size should be reported (got 0)")
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

	if _, err := ms.QueryNodeMetric(ctx, &QueryNodeMetricsReq{
		StartTimestamp: 0,
		EndTimestamp:   now.Unix() + 1,
		Num:            10,
	}); err != nil {
		t.Fatalf("QueryNodeMetric: %v", err)
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
	// Latency on a temp-dir SQLite should be sub-100ms; this guards
	// against the recorder accidentally storing zeros for everything.
	if h.Stats["add_node"].LastMs <= 0 {
		t.Fatalf("expected non-zero last_ms for add_node, got %v", h.Stats["add_node"].LastMs)
	}
}

func TestCleanupOlderThan_RemovesAndReportsCounts(t *testing.T) {
	ms := newTestStore(t)
	ctx := context.Background()

	old := time.Now().Add(-90 * 24 * time.Hour)
	fresh := time.Now()
	for _, ts := range []time.Time{old, fresh} {
		if err := ms.AddNodeMetric(ctx, &sampler.NodeMetrics{SyncTime: ts}); err != nil {
			t.Fatalf("AddNodeMetric: %v", err)
		}
	}

	res, err := ms.CleanupOlderThan(ctx, 30)
	if err != nil {
		t.Fatalf("CleanupOlderThan: %v", err)
	}
	if res.NodeDeleted != 1 {
		t.Fatalf("expected 1 node deletion, got %d", res.NodeDeleted)
	}
	h, _ := ms.Health(ctx)
	if h.NodeMetricsRows != 1 {
		t.Fatalf("expected 1 row remaining, got %d", h.NodeMetricsRows)
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
