package ms_test

import (
	"context"
	"testing"
	"time"

	"github.com/Ehco1996/ehco/internal/cmgr/ms"
	"github.com/Ehco1996/ehco/internal/metrics"
	"github.com/stretchr/testify/require"
)

// TestMetrics_SnapshotToStoreRoundTrip exercises the wiring from metrics.Snapshot()
// through to tstorage round-trip (spec §11).
func TestMetrics_SnapshotToStoreRoundTrip(t *testing.T) {
	// Use a unique label so this test is isolated from any other goroutine
	// that may share the global metrics store.
	const label = "integration-rule-roundtrip"
	const remote = "10.0.0.1:8080"

	// Populate counters via the hot-path API.
	metrics.IncConn(label, metrics.ConnTypeTCP, remote)
	metrics.AddBytes(label, metrics.ConnTypeTCP, remote, metrics.FlowTx, 1234)
	metrics.AddBytes(label, metrics.ConnTypeTCP, remote, metrics.FlowRx, 567)
	metrics.RecordHandshake(label, metrics.ConnTypeTCP, remote, 10*time.Millisecond)
	metrics.RecordHandshake(label, metrics.ConnTypeTCP, remote, 20*time.Millisecond)
	metrics.RecordPing(label, remote, "10.0.0.1", 42)

	// Take a snapshot. Other tests may have added different labels to the global
	// store, so we search for our specific rule by label rather than assuming
	// it's the only one.
	nm, allRules := metrics.Snapshot()
	require.NotNil(t, nm)

	var ourRule *ms.RuleSnapshot
	for _, rs := range allRules {
		if rs.Label == label {
			ourRule = rs
			break
		}
	}
	require.NotNil(t, ourRule, "expected snapshot to contain rule %q", label)

	// Open a temp store (no Start — watermark not needed for this test).
	dir := t.TempDir() + "/tsdb"
	store, err := ms.NewMetricsStore(dir, 0)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	// Wire the pair lister so QueryRuleMetric can enumerate series.
	store.SetPairLister(metrics.Pairs{})

	ctx := context.Background()

	// Write snapshot data into the store.
	require.NoError(t, store.AddNodeMetric(ctx, nm))
	require.NoError(t, store.AddRuleMetric(ctx, ourRule))

	// Query node metrics back.
	nodeResp, err := store.QueryNodeMetric(ctx, &ms.QueryNodeMetricsReq{
		StartTimestamp: nm.SyncTime.Add(-time.Minute).Unix(),
		EndTimestamp:   nm.SyncTime.Add(time.Minute).Unix(),
		Num:            10,
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, nodeResp.TOTAL, 1, "expected at least one node metric row")

	// Query rule metrics back.
	ruleResp, err := store.QueryRuleMetric(ctx, &ms.QueryRuleMetricsReq{
		RuleLabel:      label,
		Remote:         remote,
		StartTimestamp: ourRule.SyncTime.Add(-time.Minute).Unix(),
		EndTimestamp:   ourRule.SyncTime.Add(time.Minute).Unix(),
		Num:            10,
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, ruleResp.TOTAL, 1, "expected at least one rule metric row")

	row := ruleResp.Data[0]
	require.Equal(t, label, row.Label)
	require.Equal(t, remote, row.Remote)
	require.Equal(t, int64(42), row.PingLatency)
	require.Equal(t, int64(1), row.TCPConnectionCount)
	require.Equal(t, int64(1234), row.TCPNetworkTransmitBytes)
	// Handshake mean: (10ms + 20ms) / 2 = 15ms
	require.InDelta(t, 15, row.TCPHandshakeDuration, 1)
}
