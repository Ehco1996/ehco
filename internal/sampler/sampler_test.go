package sampler

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ehco1996/ehco/internal/store"
)

func TestNodeSampler_Sample(t *testing.T) {
	s := NewNodeSampler()
	ctx := context.Background()

	nm, err := s.Sample(ctx)
	if err != nil {
		t.Fatalf("Sample failed: %v", err)
	}
	if nm.CpuCoreCount <= 0 {
		t.Fatalf("expected positive cpu cores, got %d", nm.CpuCoreCount)
	}
	if nm.SyncTime.IsZero() {
		t.Fatalf("expected non-zero sync time")
	}

	// Second sample to compute network rate
	time.Sleep(150 * time.Millisecond)
	nm2, err := s.Sample(ctx)
	if err != nil {
		t.Fatalf("Second sample failed: %v", err)
	}
	if nm2.SyncTime.Before(nm.SyncTime) {
		t.Fatalf("expected newer sync time")
	}
}

func TestCollector_SampleOnce(t *testing.T) {
	targetDir := filepath.Join(t.TempDir(), "metrics_ts")
	st, err := store.NewStore(targetDir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer st.Close()

	s := NewNodeSampler()
	c := NewCollector(st, s)

	ctx := context.Background()
	c.sampleOnce(ctx)

	resp, err := st.QueryNodeMetric(ctx, &store.QueryNodeMetricsReq{
		StartTimestamp: 0,
		EndTimestamp:   time.Now().Unix() + 10,
	})
	if err != nil {
		t.Fatalf("QueryNodeMetric failed: %v", err)
	}
	if resp.TOTAL != 1 {
		t.Fatalf("expected 1 metric stored, got %d", resp.TOTAL)
	}
}
