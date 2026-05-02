# TSDB Replacement & Metrics Pipeline Refactor — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Spec:** [`docs/superpowers/specs/2026-05-02-tsdb-replacement-design.md`](../specs/2026-05-02-tsdb-replacement-design.md)

**Goal:** Replace SQLite-backed monitoring with `nakabonne/tstorage`; remove the Prometheus client + node_exporter integration; flatten the in-process self-scrape into direct function calls.

**Architecture:** New `internal/metrics` package owns an atomic counter store and a gopsutil node collector, exposes `Snapshot()` to cmgr. New `internal/cmgr/ms` writes snapshots into tstorage and serves time-range queries by composing per-metric `Select` calls. All Prometheus / node_exporter / self-HTTP-scrape infrastructure deleted.

**Tech Stack:** Go 1.26, `nakabonne/tstorage` (pure-Go embedded TSDB), `shirou/gopsutil/v4` (cross-platform node info).

**Why three commits:** The middle commit cannot be decomposed without keeping non-compiling intermediate states or a temporary parallel package. The metrics pipeline crosses package boundaries (transporter → metrics, cmgr → metric_reader → metrics, web → metrics), and the new API names collide with the old ones inside `internal/metrics`. Atomic cutover is the honest answer; tests cover correctness.

---

## Conventions

- **TDD inside each task.** Within Task 2, write the new package's tests first (they fail to compile), then write the implementation.
- After each task: `go build ./...` and `go test -race ./...` must pass before committing.
- The Linux build needs `-tags nofibrechannel,nomountstats` (the Makefile injects them; `make build` and `make test` are the canonical entry points). For ad-hoc `go test`/`go build` invocations on Linux, replicate the tags.
- The repo's pre-commit hook runs `go fmt` / `go vet`; do not bypass.
- All new code: `CGO_ENABLED=0` compatible (the project pins this in the Makefile).

---

## File Map

**Created:**
- `internal/cmgr/ms/schema.go` — metric / label name constants
- `internal/cmgr/ms/types.go` — Query Req/Resp + NodeSnapshot/RuleSnapshot/RemoteSnapshot
- `internal/cmgr/ms/store.go` — tstorage-backed `MetricsStore` (replaces `ms.go` + `handler.go`)
- `internal/cmgr/ms/store_test.go` — round-trip + filter tests
- `internal/cmgr/ms/watermark.go` — disk watermark goroutine
- `internal/cmgr/ms/watermark_test.go` — eviction-order test
- `internal/metrics/store.go` — atomic counter store
- `internal/metrics/api.go` — public hot-path API
- `internal/metrics/snapshot.go` — `Snapshot()` exporter + `Pairs` PairLister
- `internal/metrics/node.go` — gopsutil-based node collector (replaces `node_linux.go` + `node_darwin.go`)
- `internal/metrics/lifecycle.go` — `Start(ctx, cfg)` entrypoint
- `internal/metrics/store_test.go` — concurrent hot-path + snapshot drain tests
- `pkg/xray/bandwidth_recorder_local.go` — replaces HTTP-scrape with gopsutil read

**Modified:**
- `internal/metrics/metrics.go` — strip prometheus collectors; keep package-level conn-type constants only
- `internal/metrics/ping.go` — `OnRecv` callback writes via new `RecordPing` API
- `internal/transporter/base.go` — replace `CurConnectionCount.WithLabelValues(...).Inc/Dec()` with `metrics.IncConn/DecConn`
- `internal/transporter/raw.go`, `ws.go` — replace `HandShakeDurationMilliseconds.WithLabelValues(...).Observe(...)` with `metrics.RecordHandshake`
- `internal/conn/relay_conn.go` — replace `NetWorkTransmitBytes.WithLabelValues(...).Add(...)` with `metrics.AddBytes`
- `internal/cli/config.go` — drop `metrics.EhcoAlive.Set(...)` call
- `internal/cli/flags.go` — drop `--metric_url` flag if present
- `internal/config/config.go` — drop `GetMetricURL` method
- `internal/cmgr/config.go` — drop `MetricsURL` field
- `internal/cmgr/cmgr.go` — drop `mr` field, init metrics package, open new MetricsStore, wire PairLister, defer Close
- `internal/cmgr/syncer.go` — replace `cm.mr.ReadOnce(ctx)` with `metrics.Snapshot()`
- `internal/relay/server.go` — drop `MetricsURL: cfg.GetMetricURL()` line
- `internal/web/server.go` — drop `/metrics` route, drop `RegisterEhcoMetrics` + `RegisterNodeExporterMetrics` calls
- `pkg/xray/user.go` — call new local recorder
- `go.mod`, `go.sum`

**Deleted:**
- `internal/cmgr/ms/ms.go`, `internal/cmgr/ms/handler.go`
- `internal/metrics/node_linux.go`, `internal/metrics/node_darwin.go`
- `pkg/metric_reader/` (entire dir)
- `pkg/xray/bandwidth_recorder.go`

---

# Task 1 — Add deps

**Goal:** Pull `tstorage` and `gopsutil/v4` into the module. Existing code stays untouched and continues to compile.

- [ ] **Step 1: Get deps**

```bash
cd /Users/ehco/Developer/code/my/ehco
go get github.com/nakabonne/tstorage@latest
go get github.com/shirou/gopsutil/v4@latest
```

- [ ] **Step 2: Verify**

```bash
grep -E "nakabonne/tstorage|shirou/gopsutil" go.mod
go build ./...
```
Expected: both deps appear in `go.mod` direct requires; build clean.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore(deps): add tstorage and gopsutil for metrics refactor"
```

---

# Task 2 — Cutover: new metrics pipeline

**Goal:** Replace the entire metrics pipeline atomically. After this commit:
- `internal/metrics` is an atomic counter store + gopsutil collector with no prometheus dependency.
- `internal/cmgr/ms` is tstorage-backed.
- `cmgr.syncOnce` calls `metrics.Snapshot()` directly (no HTTP self-scrape).
- `/metrics` HTTP route is gone.
- `pkg/metric_reader/` is gone.
- `pkg/xray/bandwidth_recorder.go` reads via gopsutil, not HTTP.
- All transporter / conn / cli call sites use the new API.
- Tests pass; binary builds.

The implementation follows TDD: tests for the two new packages are written first (Steps 2 + 7), fail to compile (the implementation doesn't exist yet), then implementation makes them pass. After the new packages compile and tests pass, the call-site cutover happens (Steps 12-19) in one sweep.

## 2.A — New `internal/cmgr/ms` package

The old `ms.go` + `handler.go` are deleted; new files take their place. Tests come first.

- [ ] **Step 1: Delete old ms files**

```bash
git rm internal/cmgr/ms/ms.go internal/cmgr/ms/handler.go
```

(Note: this leaves `internal/cmgr/cmgr.go` and `internal/cmgr/syncer.go` referring to deleted symbols — they'll be fixed in 2.D. Build is broken between here and Step 19; that's expected for an atomic cutover.)

- [ ] **Step 2: Write `internal/cmgr/ms/store_test.go` (failing)**

```go
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
```

- [ ] **Step 3: Write `internal/cmgr/ms/schema.go`**

```go
package ms

const (
	MetricNodeCPU    = "node_cpu_usage"
	MetricNodeMem    = "node_memory_usage"
	MetricNodeDisk   = "node_disk_usage"
	MetricNodeNetIn  = "node_network_in"
	MetricNodeNetOut = "node_network_out"

	MetricRulePingMs      = "rule_ping_latency_ms"
	MetricRuleConnCount   = "rule_conn_count"
	MetricRuleHandshakeMs = "rule_handshake_ms"
	MetricRuleBytesTotal  = "rule_bytes_total"
)

const (
	LblLabel    = "label"
	LblRemote   = "remote"
	LblConnType = "conn_type"
	LblFlow     = "flow"
)

const (
	ConnTypeTCP = "tcp"
	ConnTypeUDP = "udp"
	FlowTx      = "tx"
	FlowRx      = "rx"
)

var nodeMetrics = []string{
	MetricNodeCPU, MetricNodeMem, MetricNodeDisk, MetricNodeNetIn, MetricNodeNetOut,
}
```

- [ ] **Step 4: Write `internal/cmgr/ms/types.go`**

```go
package ms

import "time"

// Query request/response (frontend contract — preserved from old SQLite store).

type NodeMetrics struct {
	Timestamp   int64   `json:"timestamp"`
	CPUUsage    float64 `json:"cpu_usage"`
	MemoryUsage float64 `json:"memory_usage"`
	DiskUsage   float64 `json:"disk_usage"`
	NetworkIn   float64 `json:"network_in"`
	NetworkOut  float64 `json:"network_out"`
}

type QueryNodeMetricsReq struct {
	StartTimestamp int64
	EndTimestamp   int64
	Num            int64
}

type QueryNodeMetricsResp struct {
	TOTAL int           `json:"total"`
	Data  []NodeMetrics `json:"data"`
}

type RuleMetricsData struct {
	Timestamp               int64  `json:"timestamp"`
	Label                   string `json:"label"`
	Remote                  string `json:"remote"`
	PingLatency             int64  `json:"ping_latency"`
	TCPConnectionCount      int64  `json:"tcp_connection_count"`
	TCPHandshakeDuration    int64  `json:"tcp_handshake_duration"`
	TCPNetworkTransmitBytes int64  `json:"tcp_network_transmit_bytes"`
	UDPConnectionCount      int64  `json:"udp_connection_count"`
	UDPHandshakeDuration    int64  `json:"udp_handshake_duration"`
	UDPNetworkTransmitBytes int64  `json:"udp_network_transmit_bytes"`
}

type QueryRuleMetricsReq struct {
	RuleLabel      string
	Remote         string
	StartTimestamp int64
	EndTimestamp   int64
	Num            int64
}

type QueryRuleMetricsResp struct {
	TOTAL int               `json:"total"`
	Data  []RuleMetricsData `json:"data"`
}

// Snapshot inputs (from internal/metrics).

type NodeSnapshot struct {
	SyncTime    time.Time
	CPUUsage    float64
	MemoryUsage float64
	DiskUsage   float64
	NetworkIn   float64
	NetworkOut  float64
}

type RemoteSnapshot struct {
	Remote string

	PingLatencyMs int64

	TCPConnCount int64
	UDPConnCount int64

	// Mean over snapshot interval (zero if no new handshakes).
	TCPHandshakeMs int64
	UDPHandshakeMs int64

	// Counters (monotonic since process start).
	TCPBytesTx int64
	TCPBytesRx int64
	UDPBytesTx int64
	UDPBytesRx int64
}

type RuleSnapshot struct {
	SyncTime time.Time
	Label    string
	Remotes  []RemoteSnapshot
}

// PairLister discovers known (label, remote) pairs (live index from internal/metrics).
type PairLister interface {
	Pairs(labelFilter, remoteFilter string) []LabelRemote
}

type LabelRemote struct {
	Label  string
	Remote string
}
```

- [ ] **Step 5: Write `internal/cmgr/ms/store.go`**

```go
package ms

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/nakabonne/tstorage"
	"go.uber.org/zap"
)

const writeTimeout = 0 // tstorage default

type MetricsStore struct {
	dir   string
	tsdb  tstorage.Storage
	idx   PairLister
	idxMu sync.RWMutex
	l     *zap.SugaredLogger

	wmCancel context.CancelFunc
}

// NewMetricsStore opens (or creates) tstorage at dataDir and starts the disk
// watermark goroutine bound to ctx. watermarkPct=0 selects the default (50%).
func NewMetricsStore(ctx context.Context, dataDir string, watermarkPct int) (*MetricsStore, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir tsdb dir: %w", err)
	}
	storage, err := tstorage.NewStorage(
		tstorage.WithDataPath(dataDir),
		tstorage.WithRetention(retention),
		tstorage.WithTimestampPrecision(tstorage.Seconds),
	)
	if err != nil {
		return nil, fmt.Errorf("open tstorage: %w", err)
	}

	wmCtx, cancel := context.WithCancel(ctx)
	ms := &MetricsStore{
		dir:      dataDir,
		tsdb:     storage,
		l:        zap.S().Named("ms"),
		wmCancel: cancel,
	}
	ms.startWatermark(wmCtx, watermarkPct)
	return ms, nil
}

func (ms *MetricsStore) SetPairLister(p PairLister) {
	ms.idxMu.Lock()
	ms.idx = p
	ms.idxMu.Unlock()
}

func (ms *MetricsStore) pairs(label, remote string) []LabelRemote {
	ms.idxMu.RLock()
	defer ms.idxMu.RUnlock()
	if ms.idx == nil {
		return nil
	}
	return ms.idx.Pairs(label, remote)
}

func (ms *MetricsStore) Close() error {
	if ms.wmCancel != nil {
		ms.wmCancel()
	}
	return ms.tsdb.Close()
}

func (ms *MetricsStore) DataDir() string { return ms.dir }

func (ms *MetricsStore) AddNodeMetric(ctx context.Context, s *NodeSnapshot) error {
	ts := s.SyncTime.Unix()
	return ms.tsdb.InsertRows([]tstorage.Row{
		{Metric: MetricNodeCPU, DataPoint: tstorage.DataPoint{Timestamp: ts, Value: s.CPUUsage}},
		{Metric: MetricNodeMem, DataPoint: tstorage.DataPoint{Timestamp: ts, Value: s.MemoryUsage}},
		{Metric: MetricNodeDisk, DataPoint: tstorage.DataPoint{Timestamp: ts, Value: s.DiskUsage}},
		{Metric: MetricNodeNetIn, DataPoint: tstorage.DataPoint{Timestamp: ts, Value: s.NetworkIn}},
		{Metric: MetricNodeNetOut, DataPoint: tstorage.DataPoint{Timestamp: ts, Value: s.NetworkOut}},
	})
}

func (ms *MetricsStore) AddRuleMetric(ctx context.Context, s *RuleSnapshot) error {
	ts := s.SyncTime.Unix()
	rows := make([]tstorage.Row, 0, len(s.Remotes)*9)
	for _, b := range s.Remotes {
		base := []tstorage.Label{
			{Name: LblLabel, Value: s.Label},
			{Name: LblRemote, Value: b.Remote},
		}
		rows = append(rows,
			row(MetricRulePingMs, base, ts, float64(b.PingLatencyMs)),

			row(MetricRuleConnCount, withConn(base, ConnTypeTCP), ts, float64(b.TCPConnCount)),
			row(MetricRuleConnCount, withConn(base, ConnTypeUDP), ts, float64(b.UDPConnCount)),

			row(MetricRuleHandshakeMs, withConn(base, ConnTypeTCP), ts, float64(b.TCPHandshakeMs)),
			row(MetricRuleHandshakeMs, withConn(base, ConnTypeUDP), ts, float64(b.UDPHandshakeMs)),

			row(MetricRuleBytesTotal, withConnFlow(base, ConnTypeTCP, FlowTx), ts, float64(b.TCPBytesTx)),
			row(MetricRuleBytesTotal, withConnFlow(base, ConnTypeTCP, FlowRx), ts, float64(b.TCPBytesRx)),
			row(MetricRuleBytesTotal, withConnFlow(base, ConnTypeUDP, FlowTx), ts, float64(b.UDPBytesTx)),
			row(MetricRuleBytesTotal, withConnFlow(base, ConnTypeUDP, FlowRx), ts, float64(b.UDPBytesRx)),
		)
	}
	return ms.tsdb.InsertRows(rows)
}

func (ms *MetricsStore) QueryNodeMetric(ctx context.Context, req *QueryNodeMetricsReq) (*QueryNodeMetricsResp, error) {
	type result struct {
		metric string
		points []*tstorage.DataPoint
		err    error
	}
	ch := make(chan result, len(nodeMetrics))
	for _, m := range nodeMetrics {
		go func(m string) {
			pts, err := ms.tsdb.Select(m, nil, req.StartTimestamp, req.EndTimestamp+1)
			ch <- result{m, pts, err}
		}(m)
	}

	byTS := make(map[int64]*NodeMetrics)
	for i := 0; i < len(nodeMetrics); i++ {
		r := <-ch
		if r.err != nil && !errors.Is(r.err, tstorage.ErrNoDataPoints) {
			return nil, fmt.Errorf("select %s: %w", r.metric, r.err)
		}
		for _, p := range r.points {
			nm, ok := byTS[p.Timestamp]
			if !ok {
				nm = &NodeMetrics{Timestamp: p.Timestamp}
				byTS[p.Timestamp] = nm
			}
			switch r.metric {
			case MetricNodeCPU:
				nm.CPUUsage = p.Value
			case MetricNodeMem:
				nm.MemoryUsage = p.Value
			case MetricNodeDisk:
				nm.DiskUsage = p.Value
			case MetricNodeNetIn:
				nm.NetworkIn = p.Value
			case MetricNodeNetOut:
				nm.NetworkOut = p.Value
			}
		}
	}

	out := make([]NodeMetrics, 0, len(byTS))
	for _, nm := range byTS {
		out = append(out, *nm)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp > out[j].Timestamp })
	if req.Num > 0 && int64(len(out)) > req.Num {
		out = out[:req.Num]
	}
	return &QueryNodeMetricsResp{TOTAL: len(out), Data: out}, nil
}

func (ms *MetricsStore) QueryRuleMetric(ctx context.Context, req *QueryRuleMetricsReq) (*QueryRuleMetricsResp, error) {
	pairs := ms.pairs(req.RuleLabel, req.Remote)
	if len(pairs) == 0 {
		return &QueryRuleMetricsResp{TOTAL: 0}, nil
	}

	rows := make([]RuleMetricsData, 0)
	for _, pr := range pairs {
		series, err := ms.fetchRuleSeries(pr, req.StartTimestamp, req.EndTimestamp+1)
		if err != nil {
			return nil, err
		}
		rows = append(rows, series...)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Timestamp > rows[j].Timestamp })
	if req.Num > 0 && int64(len(rows)) > req.Num {
		rows = rows[:req.Num]
	}
	return &QueryRuleMetricsResp{TOTAL: len(rows), Data: rows}, nil
}

func (ms *MetricsStore) fetchRuleSeries(pr LabelRemote, start, end int64) ([]RuleMetricsData, error) {
	base := []tstorage.Label{
		{Name: LblLabel, Value: pr.Label},
		{Name: LblRemote, Value: pr.Remote},
	}
	type query struct {
		metric string
		labels []tstorage.Label
		set    func(*RuleMetricsData, float64)
	}
	queries := []query{
		{MetricRulePingMs, base, func(r *RuleMetricsData, v float64) { r.PingLatency = int64(v) }},
		{MetricRuleConnCount, withConn(base, ConnTypeTCP), func(r *RuleMetricsData, v float64) { r.TCPConnectionCount = int64(v) }},
		{MetricRuleConnCount, withConn(base, ConnTypeUDP), func(r *RuleMetricsData, v float64) { r.UDPConnectionCount = int64(v) }},
		{MetricRuleHandshakeMs, withConn(base, ConnTypeTCP), func(r *RuleMetricsData, v float64) { r.TCPHandshakeDuration = int64(v) }},
		{MetricRuleHandshakeMs, withConn(base, ConnTypeUDP), func(r *RuleMetricsData, v float64) { r.UDPHandshakeDuration = int64(v) }},
		{MetricRuleBytesTotal, withConnFlow(base, ConnTypeTCP, FlowTx), func(r *RuleMetricsData, v float64) { r.TCPNetworkTransmitBytes = int64(v) }},
		{MetricRuleBytesTotal, withConnFlow(base, ConnTypeUDP, FlowTx), func(r *RuleMetricsData, v float64) { r.UDPNetworkTransmitBytes = int64(v) }},
	}

	type qr struct {
		idx    int
		points []*tstorage.DataPoint
		err    error
	}
	ch := make(chan qr, len(queries))
	for i, q := range queries {
		go func(i int, q query) {
			pts, err := ms.tsdb.Select(q.metric, q.labels, start, end)
			ch <- qr{i, pts, err}
		}(i, q)
	}

	byTS := make(map[int64]*RuleMetricsData)
	for i := 0; i < len(queries); i++ {
		r := <-ch
		if r.err != nil && !errors.Is(r.err, tstorage.ErrNoDataPoints) {
			return nil, fmt.Errorf("select rule %s: %w", queries[r.idx].metric, r.err)
		}
		for _, p := range r.points {
			row, ok := byTS[p.Timestamp]
			if !ok {
				row = &RuleMetricsData{Timestamp: p.Timestamp, Label: pr.Label, Remote: pr.Remote}
				byTS[p.Timestamp] = row
			}
			queries[r.idx].set(row, p.Value)
		}
	}

	out := make([]RuleMetricsData, 0, len(byTS))
	for _, row := range byTS {
		out = append(out, *row)
	}
	return out, nil
}

func row(metric string, labels []tstorage.Label, ts int64, v float64) tstorage.Row {
	return tstorage.Row{Metric: metric, Labels: labels, DataPoint: tstorage.DataPoint{Timestamp: ts, Value: v}}
}

func withConn(base []tstorage.Label, connType string) []tstorage.Label {
	out := make([]tstorage.Label, 0, len(base)+1)
	out = append(out, base...)
	out = append(out, tstorage.Label{Name: LblConnType, Value: connType})
	return out
}

func withConnFlow(base []tstorage.Label, connType, flow string) []tstorage.Label {
	out := make([]tstorage.Label, 0, len(base)+2)
	out = append(out, base...)
	out = append(out, tstorage.Label{Name: LblConnType, Value: connType})
	out = append(out, tstorage.Label{Name: LblFlow, Value: flow})
	return out
}

func LegacyDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".ehco", "metrics.db")
}
```

- [ ] **Step 6: Write `internal/cmgr/ms/watermark.go` and `watermark_test.go`**

`watermark.go`:

```go
package ms

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"go.uber.org/zap"
)

const (
	retention           = 30 * 24 * time.Hour
	watermarkInterval   = 5 * time.Minute
	defaultWatermarkPct = 50
)

type DiskUsageProber func(path string) (usedPct float64, err error)

func defaultDiskUsage(path string) (float64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	total := stat.Blocks * uint64(stat.Bsize)
	if total == 0 {
		return 0, errors.New("zero-sized filesystem")
	}
	avail := stat.Bavail * uint64(stat.Bsize)
	used := total - avail
	return float64(used) / float64(total) * 100, nil
}

func (ms *MetricsStore) startWatermark(ctx context.Context, limitPct int) {
	if limitPct <= 0 {
		limitPct = defaultWatermarkPct
	}
	go ms.runWatermark(ctx, limitPct, defaultDiskUsage)
}

func (ms *MetricsStore) runWatermark(ctx context.Context, limitPct int, prober DiskUsageProber) {
	t := time.NewTicker(watermarkInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			ms.evictUntilBelow(limitPct, prober)
		}
	}
}

func (ms *MetricsStore) evictUntilBelow(limitPct int, prober DiskUsageProber) {
	for {
		pct, err := prober(ms.dir)
		if err != nil {
			ms.logger().Warnf("watermark probe: %v", err)
			return
		}
		if pct < float64(limitPct) {
			return
		}
		victim, err := oldestPartition(ms.dir)
		if err != nil {
			ms.logger().Warnf("watermark scan: %v", err)
			return
		}
		if victim == "" {
			ms.logger().Warnf("watermark: disk %.1f%% but no partition to evict", pct)
			return
		}
		ms.logger().Warnf("watermark: disk %.1f%% > %d%%, evicting %s", pct, limitPct, victim)
		if err := os.RemoveAll(victim); err != nil {
			ms.logger().Errorf("watermark remove %s: %v", victim, err)
			return
		}
	}
}

func (ms *MetricsStore) logger() *zap.SugaredLogger {
	if ms.l != nil {
		return ms.l
	}
	return zap.NewNop().Sugar()
}

func oldestPartition(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	type p struct {
		path  string
		mtime time.Time
	}
	parts := make([]p, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		parts = append(parts, p{filepath.Join(dir, e.Name()), info.ModTime()})
	}
	if len(parts) == 0 {
		return "", nil
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i].mtime.Before(parts[j].mtime) })
	return parts[0].path, nil
}
```

`watermark_test.go`:

```go
package ms

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEvictUntilBelow_OldestFirst(t *testing.T) {
	dir := t.TempDir()
	for i, name := range []string{"old", "mid", "new"} {
		p := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(p, 0o755))
		mt := time.Now().Add(time.Duration(i) * time.Hour)
		require.NoError(t, os.Chtimes(p, mt, mt))
	}

	calls := 0
	prober := func(string) (float64, error) {
		calls++
		if calls <= 2 {
			return 80, nil
		}
		return 30, nil
	}

	ms := &MetricsStore{dir: dir}
	ms.evictUntilBelow(50, prober)

	_, err := os.Stat(filepath.Join(dir, "old"))
	require.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(dir, "mid"))
	require.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(dir, "new"))
	require.NoError(t, err)
}
```

- [ ] **Step 7: Run ms tests**

```bash
go test -race ./internal/cmgr/ms/...
```
Expected: all four tests pass. (Other packages do not yet compile — that is fixed in 2.D.)

## 2.B — New `internal/metrics` package

The old `metrics.go` (Prometheus collectors) and the platform-specific `node_*.go` files are replaced. `ping.go` is rewired.

- [ ] **Step 8: Delete old platform files**

```bash
git rm internal/metrics/node_linux.go internal/metrics/node_darwin.go
```

- [ ] **Step 9: Replace `internal/metrics/metrics.go`**

The new file holds only the conn-type / flow constants used by transporter and conn:

```go
package metrics

// Public conn-type / flow constants. Re-exported with both old (METRIC_*) and
// new (ConnType*, Flow*) names so call sites can migrate at any pace.
const (
	METRIC_CONN_TYPE_TCP = ConnTypeTCP
	METRIC_CONN_TYPE_UDP = ConnTypeUDP
	METRIC_FLOW_READ     = FlowRx
	METRIC_FLOW_WRITE    = FlowTx
)
```

- [ ] **Step 10: Write `internal/metrics/store.go`**

```go
package metrics

import (
	"math"
	"sort"
	"sync"
	"sync/atomic"
)

var globalStore = newStore()

type store struct {
	mu    sync.RWMutex
	rules map[string]*ruleBucket

	nodeCPU atomic.Uint64 // float64 bits
	nodeMem atomic.Uint64
	nodeDsk atomic.Uint64
	nodeIn  atomic.Uint64
	nodeOut atomic.Uint64
}

type ruleBucket struct {
	label   string
	mu      sync.RWMutex
	remotes map[string]*remoteBucket
}

type remoteBucket struct {
	tcpConn atomic.Int64
	udpConn atomic.Int64

	tcpBytesTx atomic.Int64
	tcpBytesRx atomic.Int64
	udpBytesTx atomic.Int64
	udpBytesRx atomic.Int64

	tcpHsSum atomic.Int64
	tcpHsCnt atomic.Int64
	udpHsSum atomic.Int64
	udpHsCnt atomic.Int64

	pingLatencyMs atomic.Int64
	pingTargetIP  atomic.Pointer[string]
}

func newStore() *store { return &store{rules: make(map[string]*ruleBucket)} }

func (s *store) getOrCreateRemote(label, remote string) *remoteBucket {
	s.mu.RLock()
	rb := s.rules[label]
	s.mu.RUnlock()
	if rb == nil {
		s.mu.Lock()
		if rb = s.rules[label]; rb == nil {
			rb = &ruleBucket{label: label, remotes: make(map[string]*remoteBucket)}
			s.rules[label] = rb
		}
		s.mu.Unlock()
	}

	rb.mu.RLock()
	rem := rb.remotes[remote]
	rb.mu.RUnlock()
	if rem == nil {
		rb.mu.Lock()
		if rem = rb.remotes[remote]; rem == nil {
			rem = &remoteBucket{}
			rb.remotes[remote] = rem
		}
		rb.mu.Unlock()
	}
	return rem
}

func (s *store) setNode(cpu, mem, disk, netIn, netOut float64) {
	s.nodeCPU.Store(math.Float64bits(cpu))
	s.nodeMem.Store(math.Float64bits(mem))
	s.nodeDsk.Store(math.Float64bits(disk))
	s.nodeIn.Store(math.Float64bits(netIn))
	s.nodeOut.Store(math.Float64bits(netOut))
}

type labelRemote struct {
	Label  string
	Remote string
}

func (s *store) listPairs(labelFilter, remoteFilter string) []labelRemote {
	s.mu.RLock()
	rules := make([]*ruleBucket, 0, len(s.rules))
	for _, rb := range s.rules {
		if labelFilter != "" && rb.label != labelFilter {
			continue
		}
		rules = append(rules, rb)
	}
	s.mu.RUnlock()

	out := make([]labelRemote, 0, len(rules))
	for _, rb := range rules {
		rb.mu.RLock()
		for remote := range rb.remotes {
			if remoteFilter != "" && remote != remoteFilter {
				continue
			}
			out = append(out, labelRemote{Label: rb.label, Remote: remote})
		}
		rb.mu.RUnlock()
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Label != out[j].Label {
			return out[i].Label < out[j].Label
		}
		return out[i].Remote < out[j].Remote
	})
	return out
}
```

- [ ] **Step 11: Write `internal/metrics/api.go`**

```go
package metrics

import "time"

const (
	ConnTypeTCP = "tcp"
	ConnTypeUDP = "udp"
	FlowTx      = "tx"
	FlowRx      = "rx"
)

func IncConn(label, connType, remote string) {
	rb := globalStore.getOrCreateRemote(label, remote)
	switch connType {
	case ConnTypeTCP:
		rb.tcpConn.Add(1)
	case ConnTypeUDP:
		rb.udpConn.Add(1)
	}
}

func DecConn(label, connType, remote string) {
	rb := globalStore.getOrCreateRemote(label, remote)
	switch connType {
	case ConnTypeTCP:
		rb.tcpConn.Add(-1)
	case ConnTypeUDP:
		rb.udpConn.Add(-1)
	}
}

func AddBytes(label, connType, remote, flow string, n int64) {
	rb := globalStore.getOrCreateRemote(label, remote)
	switch connType {
	case ConnTypeTCP:
		if flow == FlowTx {
			rb.tcpBytesTx.Add(n)
		} else {
			rb.tcpBytesRx.Add(n)
		}
	case ConnTypeUDP:
		if flow == FlowTx {
			rb.udpBytesTx.Add(n)
		} else {
			rb.udpBytesRx.Add(n)
		}
	}
}

func RecordHandshake(label, connType, remote string, dur time.Duration) {
	rb := globalStore.getOrCreateRemote(label, remote)
	ms := dur.Milliseconds()
	switch connType {
	case ConnTypeTCP:
		rb.tcpHsSum.Add(ms)
		rb.tcpHsCnt.Add(1)
	case ConnTypeUDP:
		rb.udpHsSum.Add(ms)
		rb.udpHsCnt.Add(1)
	}
}

func RecordPing(label, remote, ip string, latencyMs int64) {
	rb := globalStore.getOrCreateRemote(label, remote)
	rb.pingLatencyMs.Store(latencyMs)
	ipCopy := ip
	rb.pingTargetIP.Store(&ipCopy)
}
```

- [ ] **Step 12: Write `internal/metrics/snapshot.go`**

```go
package metrics

import (
	"math"
	"sync/atomic"
	"time"

	"github.com/Ehco1996/ehco/internal/cmgr/ms"
)

func Snapshot() (*ms.NodeSnapshot, []*ms.RuleSnapshot) {
	now := time.Now()
	node := &ms.NodeSnapshot{
		SyncTime:    now,
		CPUUsage:    math.Float64frombits(globalStore.nodeCPU.Load()),
		MemoryUsage: math.Float64frombits(globalStore.nodeMem.Load()),
		DiskUsage:   math.Float64frombits(globalStore.nodeDsk.Load()),
		NetworkIn:   math.Float64frombits(globalStore.nodeIn.Load()),
		NetworkOut:  math.Float64frombits(globalStore.nodeOut.Load()),
	}

	globalStore.mu.RLock()
	rules := make([]*ms.RuleSnapshot, 0, len(globalStore.rules))
	for _, rb := range globalStore.rules {
		rules = append(rules, snapshotRule(rb, now))
	}
	globalStore.mu.RUnlock()
	return node, rules
}

func snapshotRule(rb *ruleBucket, now time.Time) *ms.RuleSnapshot {
	rb.mu.RLock()
	remotes := make([]ms.RemoteSnapshot, 0, len(rb.remotes))
	for remote, b := range rb.remotes {
		remotes = append(remotes, ms.RemoteSnapshot{
			Remote:         remote,
			PingLatencyMs:  b.pingLatencyMs.Load(),
			TCPConnCount:   b.tcpConn.Load(),
			UDPConnCount:   b.udpConn.Load(),
			TCPHandshakeMs: drainMean(&b.tcpHsSum, &b.tcpHsCnt),
			UDPHandshakeMs: drainMean(&b.udpHsSum, &b.udpHsCnt),
			TCPBytesTx:     b.tcpBytesTx.Load(),
			TCPBytesRx:     b.tcpBytesRx.Load(),
			UDPBytesTx:     b.udpBytesTx.Load(),
			UDPBytesRx:     b.udpBytesRx.Load(),
		})
	}
	rb.mu.RUnlock()
	return &ms.RuleSnapshot{SyncTime: now, Label: rb.label, Remotes: remotes}
}

func drainMean(sum, cnt *atomic.Int64) int64 {
	s := sum.Swap(0)
	c := cnt.Swap(0)
	if c == 0 {
		return 0
	}
	return s / c
}

// Pairs implements ms.PairLister.
type Pairs struct{}

func (Pairs) Pairs(labelFilter, remoteFilter string) []ms.LabelRemote {
	live := globalStore.listPairs(labelFilter, remoteFilter)
	out := make([]ms.LabelRemote, len(live))
	for i, lr := range live {
		out[i] = ms.LabelRemote{Label: lr.Label, Remote: lr.Remote}
	}
	return out
}
```

- [ ] **Step 13: Write `internal/metrics/node.go`**

```go
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
```

- [ ] **Step 14: Write `internal/metrics/lifecycle.go`**

```go
package metrics

import (
	"context"

	"github.com/Ehco1996/ehco/internal/config"
	"go.uber.org/zap"
)

// Start launches background collectors. Call once per process.
func Start(ctx context.Context, cfg *config.Config) {
	l := zap.S().Named("metrics")
	go startNodeCollector(ctx, l)
	if cfg.EnablePing {
		pg := NewPingGroup(cfg)
		go pg.Run()
	}
}
```

- [ ] **Step 15: Edit `internal/metrics/ping.go`**

Replace the `pinger.OnRecv` block (current lines 32-38) with:

```go
	pinger.OnRecv = func(pkt *ping.Packet) {
		ip := pkt.IPAddr.String()
		RecordPing(ruleLabel, remote, ip, pkt.Rtt.Milliseconds())
		pg.logger.Sugar().Infof("%d bytes from %s icmp_seq=%d time=%v ttl=%v",
			pkt.Nbytes, pkt.Addr, pkt.Seq, pkt.Rtt, pkt.Ttl)
	}
```

- [ ] **Step 16: Write `internal/metrics/store_test.go`**

```go
package metrics

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHotPath_ConcurrentWriters(t *testing.T) {
	globalStore = newStore()
	const writers = 50
	const writesPer = 1000

	var wg sync.WaitGroup
	wg.Add(writers)
	for i := 0; i < writers; i++ {
		go func(i int) {
			defer wg.Done()
			label := "rule" + string(rune('A'+i%5))
			remote := "10.0.0." + string(rune('1'+i%9))
			for j := 0; j < writesPer; j++ {
				IncConn(label, ConnTypeTCP, remote)
				AddBytes(label, ConnTypeTCP, remote, FlowTx, 100)
				AddBytes(label, ConnTypeTCP, remote, FlowRx, 50)
				RecordHandshake(label, ConnTypeTCP, remote, 5*time.Millisecond)
				DecConn(label, ConnTypeTCP, remote)
			}
		}(i)
	}
	wg.Wait()

	_, rules := Snapshot()

	var totalConn, totalTx, totalRx int64
	for _, rs := range rules {
		for _, b := range rs.Remotes {
			totalConn += b.TCPConnCount
			totalTx += b.TCPBytesTx
			totalRx += b.TCPBytesRx
		}
	}
	require.Equal(t, int64(0), totalConn) // Inc/Dec cancel
	require.Equal(t, int64(writers*writesPer*100), totalTx)
	require.Equal(t, int64(writers*writesPer*50), totalRx)
}

func TestSnapshot_DrainsHandshakeMean(t *testing.T) {
	globalStore = newStore()
	RecordHandshake("r", ConnTypeTCP, "x", 10*time.Millisecond)
	RecordHandshake("r", ConnTypeTCP, "x", 20*time.Millisecond)

	_, rules := Snapshot()
	require.Len(t, rules, 1)
	require.Equal(t, int64(15), rules[0].Remotes[0].TCPHandshakeMs)

	_, rules = Snapshot()
	require.Equal(t, int64(0), rules[0].Remotes[0].TCPHandshakeMs)
}

func TestPairs_Filtering(t *testing.T) {
	globalStore = newStore()
	IncConn("a", ConnTypeTCP, "r1")
	IncConn("a", ConnTypeTCP, "r2")
	IncConn("b", ConnTypeTCP, "r1")

	require.Len(t, Pairs{}.Pairs("", ""), 3)
	require.Len(t, Pairs{}.Pairs("a", ""), 2)
	require.Len(t, Pairs{}.Pairs("", "r1"), 2)
	require.Len(t, Pairs{}.Pairs("a", "r1"), 1)
}
```

## 2.C — Rewire all call sites

The new metrics API exists. Now all call sites must switch off the deleted Prometheus collectors. Build is currently broken; this section makes it green.

- [ ] **Step 17: Update `internal/transporter/base.go`**

In `RelayTCPConn`, replace the metrics-related block with:

```go
metrics.IncConn(b.cfg.Label, metrics.ConnTypeTCP, remote.Address)
defer metrics.DecConn(b.cfg.Label, metrics.ConnTypeTCP, remote.Address)
```

In `RelayUDPConn`, the same with `ConnTypeUDP`. Drop the `labels := []string{...}` lines that were only needed for Prometheus' `WithLabelValues`.

- [ ] **Step 18: Update `internal/transporter/raw.go` and `ws.go`**

In each, replace the line that calls `metrics.HandShakeDurationMilliseconds.WithLabelValues(labels...).Observe(...)` with:

```go
metrics.RecordHandshake(b.cfg.Label, metrics.ConnTypeTCP, remote.Address, latency)
```

(Both `raw` and `ws` transports operate on TCP at the relay layer; UDP goes through a different path.) Inspect the surrounding code with `grep -B 5 HandShakeDurationMilliseconds internal/transporter/{raw,ws}.go` to confirm variable names before editing.

- [ ] **Step 19: Update `internal/conn/relay_conn.go`**

Around lines 200-210 there are two `metrics.NetWorkTransmitBytes.WithLabelValues(labels...).Add(...)` calls (one for read flow, one for write flow). Replace each with the matching `metrics.AddBytes(label, connType, remote, flow, int64(n))` call. Use `metrics.FlowRx` for the read-side accumulator and `metrics.FlowTx` for the write-side. Read the function with:

```bash
sed -n '180,220p' internal/conn/relay_conn.go
```

to identify the local variables holding `label`, `connType`, and `remote` (they are in scope as part of the existing `labels` slice construction).

- [ ] **Step 20: Update `internal/cli/config.go`**

Read and edit lines around 90-105:

```bash
sed -n '85,105p' internal/cli/config.go
```

Delete the line `metrics.EhcoAlive.Set(metrics.EhcoAliveStateRunning)`. If the surrounding `metrics` import becomes unused, drop it.

- [ ] **Step 21: Update `internal/cli/flags.go` and `internal/config/config.go`**

```bash
grep -n "metric_url\|MetricURL\|GetMetricURL" internal/cli/flags.go internal/config/config.go
```

Delete the `--metric_url` flag definition (if present) and the `GetMetricURL()` method on `*Config`. Remove the underlying field if it has no other consumer.

- [ ] **Step 22: Update `internal/cmgr/config.go`**

Replace the file contents (preserving any other fields):

```go
package cmgr

type Config struct {
	SyncInterval        int
	SyncURL             string
	MaxDiskUsagePercent int // 0 = use default (50)
}

func (c *Config) NeedSync() bool {
	return c.SyncURL != ""
}

func (c *Config) NeedMetrics() bool {
	return c.SyncInterval > 0
}
```

- [ ] **Step 23: Update `internal/relay/server.go`**

Drop the `MetricsURL: cfg.GetMetricURL(),` line from the `cmgr.Config` literal at line ~35. If `MaxDiskUsagePercent` should be plumbed through, add `MaxDiskUsagePercent: cfg.GetMaxDiskUsagePercent(),` and a corresponding accessor in `internal/config/config.go` (default 0). For now, leave it at zero — `NewMetricsStore` selects the default 50%.

- [ ] **Step 24: Update `internal/cmgr/cmgr.go`**

The cmgr lifecycle now opens the new `MetricsStore`, wires the `Pairs` index, defers `Close`, and drops `metric_reader`. Read the file in full first (`internal/cmgr/cmgr.go`) and apply these edits:

- Remove import `"github.com/Ehco1996/ehco/pkg/metric_reader"`.
- Add imports `"github.com/Ehco1996/ehco/internal/metrics"`.
- Remove field `mr metric_reader.Reader` from `cmgrImpl`.
- Replace the `if cfg.NeedMetrics()` block in `NewCmgr` with:

```go
if cfg.NeedMetrics() {
	homeDir, _ := os.UserHomeDir()
	dataDir := filepath.Join(homeDir, ".ehco", "tsdb")
	store, err := ms.NewMetricsStore(context.Background(), dataDir, cfg.MaxDiskUsagePercent)
	if err != nil {
		return nil, err
	}
	store.SetPairLister(metrics.Pairs{})
	cmgr.ms = store

	if legacy := ms.LegacyDBPath(); legacy != "" {
		if _, err := os.Stat(legacy); err == nil {
			cmgr.l.Warnf("legacy SQLite metrics db at %s is no longer used and can be safely deleted", legacy)
		}
	}
}
```

(Note: passing `context.Background()` to `NewMetricsStore` makes the watermark goroutine outlive `Start`'s ctx. This is intentional — the watermark must keep running across reloads. Cancellation happens via `Close()`.)

- In `Start`, prepend a deferred close of `cm.ms`:

```go
func (cm *cmgrImpl) Start(ctx context.Context, errCH chan error) {
	defer func() {
		if cm.ms != nil {
			if err := cm.ms.Close(); err != nil {
				cm.l.Errorf("close metrics store: %v", err)
			}
		}
	}()
	cm.l.Infof("Start Cmgr sync interval=%d", cm.cfg.SyncInterval)
	// ...rest unchanged
}
```

- Replace `QueryNodeMetrics` and `QueryRuleMetrics` to drop the `cm.mr.ReadOnce` refresh path:

```go
func (cm *cmgrImpl) QueryNodeMetrics(ctx context.Context, req *ms.QueryNodeMetricsReq, refresh bool) (*ms.QueryNodeMetricsResp, error) {
	if refresh {
		cm.snapshotOnce(ctx)
	}
	return cm.ms.QueryNodeMetric(ctx, req)
}

func (cm *cmgrImpl) QueryRuleMetrics(ctx context.Context, req *ms.QueryRuleMetricsReq, refresh bool) (*ms.QueryRuleMetricsResp, error) {
	if refresh {
		cm.snapshotOnce(ctx)
	}
	return cm.ms.QueryRuleMetric(ctx, req)
}

func (cm *cmgrImpl) snapshotOnce(ctx context.Context) {
	nm, rules := metrics.Snapshot()
	if err := cm.ms.AddNodeMetric(ctx, nm); err != nil {
		cm.l.Errorf("snapshot AddNodeMetric: %v", err)
	}
	for _, rs := range rules {
		if err := cm.ms.AddRuleMetric(ctx, rs); err != nil {
			cm.l.Errorf("snapshot AddRuleMetric: %v", err)
		}
	}
}
```

- [ ] **Step 25: Update `internal/cmgr/syncer.go`**

Drop the `metric_reader` import. Replace the `syncReq.Node` field type with a local struct so the upstream sync JSON shape is preserved:

```go
type syncNodeMetrics struct {
	Timestamp   int64   `json:"timestamp"`
	CPUUsage    float64 `json:"cpu_usage"`
	MemoryUsage float64 `json:"memory_usage"`
	DiskUsage   float64 `json:"disk_usage"`
	NetworkIn   float64 `json:"network_in"`
	NetworkOut  float64 `json:"network_out"`
}

type syncReq struct {
	Version VersionInfo      `json:"version"`
	Node    syncNodeMetrics  `json:"node"`
	Stats   []StatsPerRule   `json:"stats"`
}
```

Replace the `if cm.cfg.NeedMetrics()` block in `syncOnce` with:

```go
if cm.cfg.NeedMetrics() {
	nm, rules := metrics.Snapshot()
	req.Node = syncNodeMetrics{
		Timestamp:   nm.SyncTime.Unix(),
		CPUUsage:    nm.CPUUsage,
		MemoryUsage: nm.MemoryUsage,
		DiskUsage:   nm.DiskUsage,
		NetworkIn:   nm.NetworkIn,
		NetworkOut:  nm.NetworkOut,
	}
	if err := cm.ms.AddNodeMetric(ctx, nm); err != nil {
		cm.l.Errorf("add node metric: %v", err)
	}
	for _, rs := range rules {
		if err := cm.ms.AddRuleMetric(ctx, rs); err != nil {
			cm.l.Errorf("add rule metric: %v", err)
		}
	}
}
```

Add `"github.com/Ehco1996/ehco/internal/metrics"` to imports.

- [ ] **Step 26: Update `internal/web/server.go`**

```bash
grep -n "metricsPath\|RegisterEhcoMetrics\|RegisterNodeExporterMetrics\|promhttp" internal/web/server.go
```

Apply:
- Remove import `"github.com/prometheus/client_golang/prometheus/promhttp"`.
- Remove the `metricsPath = "/metrics/"` constant.
- Remove the `metrics.RegisterEhcoMetrics(cfg)` call and its error wrapper.
- Remove the `metrics.RegisterNodeExporterMetrics(cfg)` call and its error wrapper.
- Remove the route registration `e.GET(metricsPath, echo.WrapHandler(promhttp.Handler()))`.
- Add `metrics.Start(ctx, cfg)` somewhere in the startup path (after cmgr is ready, before serving). If the function signature doesn't carry `ctx` to that point, propagate it from the caller (`cli.MustStartComponents`).

- [ ] **Step 27: Replace `pkg/xray/bandwidth_recorder.go`**

```bash
git rm pkg/xray/bandwidth_recorder.go
```

Create `pkg/xray/bandwidth_recorder_local.go`:

```go
package xray

import (
	"context"
	"time"

	"github.com/shirou/gopsutil/v4/net"
)

type bandwidthRecorder struct {
	prevSend uint64
	prevRecv uint64
	primed   bool
}

func NewBandwidthRecorder() *bandwidthRecorder { return &bandwidthRecorder{} }

func (b *bandwidthRecorder) RecordOnce(ctx context.Context) (uploadIncr, downloadIncr float64, err error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	io, err := net.IOCountersWithContext(ctx, false)
	if err != nil {
		return 0, 0, err
	}
	if len(io) == 0 {
		return 0, 0, nil
	}

	send := io[0].BytesSent
	recv := io[0].BytesRecv

	if !b.primed {
		b.prevSend, b.prevRecv = send, recv
		b.primed = true
		return 0, 0, nil
	}
	if send >= b.prevSend {
		uploadIncr = float64(send - b.prevSend)
	}
	if recv >= b.prevRecv {
		downloadIncr = float64(recv - b.prevRecv)
	}
	b.prevSend, b.prevRecv = send, recv
	return
}
```

Edit `pkg/xray/user.go:143`: change `up.br = NewBandwidthRecorder(metricURL)` to `up.br = NewBandwidthRecorder()`. Trace the `metricURL` parameter upward and remove it from the function signature and its caller chain when it becomes unused. Use `grep -n "NewBandwidthRecorder" pkg/xray/` to find all sites.

- [ ] **Step 28: Delete `pkg/metric_reader/`**

```bash
grep -rn '"github.com/Ehco1996/ehco/pkg/metric_reader"' .
```

Expected: no remaining imports. Then:

```bash
git rm -r pkg/metric_reader/
```

## 2.D — Verify the cutover

- [ ] **Step 29: Build everything**

```bash
go build ./...
```
Expected: clean build. If any reference to a removed symbol remains, fix it inline before continuing.

- [ ] **Step 30: Run tests with race detector**

```bash
go test -race ./...
```
Expected: all tests pass. Pay attention to:
- `internal/cmgr/ms/...` (Steps 2, 6 — round-trip, filters, watermark eviction)
- `internal/metrics/...` (Step 16 — concurrent stress, snapshot drain, pair filtering)
- `internal/transporter/...`, `internal/conn/...` if they had tests (sniff_test.go etc.) — they should still pass; the metrics calls are no-ops in tests since no rule label matches.

- [ ] **Step 31: Stage everything and commit**

```bash
git add -A
git status   # sanity-check the file list before commit
git commit -m "refactor(metrics): replace SQLite + Prometheus pipeline with embedded TSDB and atomic counters

- Replace ms.MetricsStore SQLite backend with nakabonne/tstorage; per-metric series
  composed via labels; query path fans out per-metric Selects and recombines wide
  rows (preserves the /api/v1/{node,rule}_metrics contract).
- Replace internal/metrics Prometheus collectors with an atomic counter store +
  gopsutil node collector; new hot-path API (IncConn/DecConn/AddBytes/
  RecordHandshake/RecordPing) and a Snapshot exporter consumed directly by cmgr.
- Drop the in-process /metrics endpoint, RegisterEhcoMetrics, and the
  prometheus/node_exporter integration. cmgr no longer self-HTTP-scrapes.
- Replace pkg/xray bandwidth_recorder with a gopsutil-backed local reader.
- Add disk watermark guard (default 50%) on top of tstorage's 30d retention.
- Drop --metric_url flag and config.GetMetricURL.
- Tests cover round-trip, filters, watermark eviction order, concurrent hot-path
  writers, and snapshot draining of running-mean accumulators.

Spec: docs/superpowers/specs/2026-05-02-tsdb-replacement-design.md"
```

---

# Task 3 — Drop unused deps

**Goal:** Now that no source file imports `modernc.org/sqlite`, `prometheus/*`, or `prometheus/node_exporter`, the module file can be tidied. `alecthomas/kingpin/v2` was only used by `internal/metrics/node_linux.go` (deleted) and may also become removable.

- [ ] **Step 1: Tidy**

```bash
go mod tidy
```

- [ ] **Step 2: Verify removals**

```bash
grep -E "modernc\.org/sqlite|prometheus/(client_golang|client_model|common|node_exporter)" go.mod
```
Expected: no output. (`prometheus/*` may remain under `// indirect` if a different transitive dep references them — that's acceptable; we removed the direct usage.)

```bash
grep -E "alecthomas/kingpin" go.mod
```
If it's gone too, great. If still listed, leave it — some other transitive dep needs it.

- [ ] **Step 3: Verify additions**

```bash
grep -E "nakabonne/tstorage|shirou/gopsutil" go.mod
```
Expected: both present in the direct require block.

- [ ] **Step 4: Final build + test**

```bash
make build
make test
```
Expected: clean build, all tests pass.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum
git commit -m "chore(deps): drop sqlite + prometheus + node_exporter; tidy"
```

---

# Smoke verification (no commit)

After Task 3, sanity-check end-to-end:

```bash
# Build
make build

# Touch a fake legacy db to verify the warning fires once
mkdir -p ~/.ehco && touch ~/.ehco/metrics.db

# Run against a localdev config (in another shell)
./dist/ehco -c localdev/node-1.json
# look for log line: "legacy SQLite metrics db at ... is no longer used and can be safely deleted"
# look for: ~/.ehco/tsdb/ directory created with one or more partition subdirectories after a few minutes
# Ctrl-C, then:
rm ~/.ehco/metrics.db
ls ~/.ehco/tsdb/   # should show partition dirs
```

Hit the API endpoints:

```bash
curl -s 'http://127.0.0.1:9000/api/v1/node_metrics/?start_ts=...&end_ts=...&num=100' | head
curl -s 'http://127.0.0.1:9000/api/v1/rule_metrics/?start_ts=...&end_ts=...&num=100' | head
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:9000/metrics
# expect 404 — endpoint is gone
```

---

## Self-Review

- **Spec coverage:** §1-§5 of the spec map onto Task 2.A (ms package). §6-§7 onto Task 2.B (metrics package) plus Task 2.C Steps 24-25 (cmgr query path). §8 onto Task 2.A Steps 5-6 (config + watermark). §9 onto Task 2.C Steps 22, 24, 26 (lifecycle wiring). §10 covered by tests in Steps 2, 6, 16. §11 testing strategy fully realized. §12 implementation order is the structure of Task 2.
- **Type consistency check:** `MetricsStore`, `NodeSnapshot`, `RuleSnapshot`, `RemoteSnapshot`, `LabelRemote`, `PairLister`, `Pairs` are spelled identically in every appearance. `IncConn` / `DecConn` / `AddBytes` / `RecordHandshake` / `RecordPing` / `Snapshot` / `Start` are the only public functions in `internal/metrics`.
- **No placeholders:** every step contains executable code or commands.
- **The sed/grep "go inspect first" steps in 2.C** are deliberate — those files have surrounding context that's tedious to fully reproduce inline; the engineer must read the function before substituting. Each such step names exactly what to find and what to replace it with.

---

## Plan complete — saved to `docs/superpowers/plans/2026-05-02-tsdb-replacement.md`.

**Two execution options:**

1. **Subagent-Driven (recommended for Task 2)** — Task 2 is large; dispatching subagents per sub-section (2.A, 2.B, 2.C) keeps the main session's context lean and gives explicit checkpoints between the new-package work and the cutover work.
2. **Inline Execution** — Single session does Tasks 1 → 2 → 3 in order, with a checkpoint after each commit.

Pick based on appetite for context isolation vs. speed.
