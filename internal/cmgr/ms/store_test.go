package ms_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ehco1996/ehco/internal/cmgr/ms"
	"github.com/stretchr/testify/require"
)

type fakeIdx struct{ pairs []ms.LabelRemote }

func (f *fakeIdx) Pairs(label, remote string) []ms.LabelRemote {
	out := make([]ms.LabelRemote, 0, len(f.pairs))
	for _, p := range f.pairs {
		if label != "" && p.Label != label {
			continue
		}
		if remote != "" && p.Remote != remote {
			continue
		}
		out = append(out, p)
	}
	return out
}

func newStore(t *testing.T) *ms.MetricsStore {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "tsdb")
	store, err := ms.NewMetricsStore(context.Background(), dir, 0)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestStore_NodeRoundTrip(t *testing.T) {
	store := newStore(t)
	now := time.Now()
	ctx := context.Background()

	require.NoError(t, store.AddNodeMetric(ctx, &ms.NodeSnapshot{
		SyncTime: now.Add(-60 * time.Second), CPUUsage: 10, MemoryUsage: 20, DiskUsage: 30, NetworkIn: 100, NetworkOut: 200,
	}))
	require.NoError(t, store.AddNodeMetric(ctx, &ms.NodeSnapshot{
		SyncTime: now, CPUUsage: 11, MemoryUsage: 21, DiskUsage: 31, NetworkIn: 101, NetworkOut: 201,
	}))

	resp, err := store.QueryNodeMetric(ctx, &ms.QueryNodeMetricsReq{
		StartTimestamp: now.Add(-2 * time.Minute).Unix(),
		EndTimestamp:   now.Add(time.Minute).Unix(),
		Num:            10,
	})
	require.NoError(t, err)
	require.Equal(t, 2, resp.TOTAL)
	require.Equal(t, now.Unix(), resp.Data[0].Timestamp) // DESC
	require.InDelta(t, 11.0, resp.Data[0].CPUUsage, 0.001)
	require.InDelta(t, 201.0, resp.Data[0].NetworkOut, 0.001)
	require.InDelta(t, 10.0, resp.Data[1].CPUUsage, 0.001)
}

func TestStore_RuleRoundTrip(t *testing.T) {
	store := newStore(t)
	store.SetPairLister(&fakeIdx{pairs: []ms.LabelRemote{{Label: "rule1", Remote: "1.2.3.4:80"}}})
	now := time.Now()
	ctx := context.Background()

	require.NoError(t, store.AddRuleMetric(ctx, &ms.RuleSnapshot{
		SyncTime: now,
		Label:    "rule1",
		Remotes: []ms.RemoteSnapshot{{
			Remote: "1.2.3.4:80", PingLatencyMs: 42,
			TCPConnCount: 3, UDPConnCount: 1,
			TCPHandshakeMs: 12, UDPHandshakeMs: 8,
			TCPBytesTx: 1000, TCPBytesRx: 2000, UDPBytesTx: 500, UDPBytesRx: 600,
		}},
	}))

	resp, err := store.QueryRuleMetric(ctx, &ms.QueryRuleMetricsReq{
		RuleLabel: "rule1",
		StartTimestamp: now.Add(-time.Minute).Unix(),
		EndTimestamp:   now.Add(time.Minute).Unix(),
		Num: 10,
	})
	require.NoError(t, err)
	require.Equal(t, 1, resp.TOTAL)
	row := resp.Data[0]
	require.Equal(t, "rule1", row.Label)
	require.Equal(t, "1.2.3.4:80", row.Remote)
	require.Equal(t, int64(42), row.PingLatency)
	require.Equal(t, int64(3), row.TCPConnectionCount)
	require.Equal(t, int64(12), row.TCPHandshakeDuration)
	require.Equal(t, int64(1000), row.TCPNetworkTransmitBytes) // tx only — matches old "transmit" semantics
	require.Equal(t, int64(500), row.UDPNetworkTransmitBytes)
}

func TestStore_RuleQueryFilters(t *testing.T) {
	store := newStore(t)
	store.SetPairLister(&fakeIdx{pairs: []ms.LabelRemote{
		{Label: "a", Remote: "r1"}, {Label: "a", Remote: "r2"},
		{Label: "b", Remote: "r1"}, {Label: "b", Remote: "r2"},
	}})
	now := time.Now()
	ctx := context.Background()

	for _, lbl := range []string{"a", "b"} {
		for _, rem := range []string{"r1", "r2"} {
			require.NoError(t, store.AddRuleMetric(ctx, &ms.RuleSnapshot{
				SyncTime: now, Label: lbl,
				Remotes: []ms.RemoteSnapshot{{Remote: rem, TCPConnCount: 1}},
			}))
		}
	}

	for _, tc := range []struct {
		name           string
		req            ms.QueryRuleMetricsReq
		wantRows       int
	}{
		{"no filter", ms.QueryRuleMetricsReq{Num: 100}, 4},
		{"label only", ms.QueryRuleMetricsReq{RuleLabel: "a", Num: 100}, 2},
		{"label + remote", ms.QueryRuleMetricsReq{RuleLabel: "a", Remote: "r1", Num: 100}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.req.StartTimestamp = now.Add(-time.Minute).Unix()
			tc.req.EndTimestamp = now.Add(time.Minute).Unix()
			resp, err := store.QueryRuleMetric(ctx, &tc.req)
			require.NoError(t, err)
			require.Equal(t, tc.wantRows, resp.TOTAL)
		})
	}
}
