# TSDB Replacement & Metrics Pipeline Refactor

**Date:** 2026-05-02
**Status:** Approved for implementation
**Scope:** Replace SQLite-backed `MetricsStore` with embedded TSDB; remove Prometheus client + node_exporter integration; flatten the in-process self-scrape pipeline.

---

## 1. Motivation

The current monitoring storage uses SQLite (`modernc.org/sqlite`) with two wide tables (`node_metrics`, `rule_metrics`). Pain points:

- **Query latency** under wide time windows is the primary bottleneck. Wide rows + `ORDER BY ts DESC LIMIT N` + B-tree on a single primary key do not scale to high-frequency sampling.
- **Sampling will get denser**. The next milestone is connection-level monitoring; sample frequency goes up materially.
- **Self-scrape pipeline is convoluted.** Runtime pushes counters into a Prometheus registry, then the same process HTTP-scrapes its own `/metrics`, parses Prometheus exposition text via `expfmt`, then writes to SQLite. The whole HTTP round-trip exists only to extract a snapshot of in-memory counters.
- **`prometheus/node_exporter` is a heavy dependency** with platform-specific behavior (no-op on darwin), and its registration relies on a kingpin side-effect (`kingpin.CommandLine.Parse([]string{})`).

**Goal:** Single ehco process on a single VPS handles all monitoring storage and serving — no Prometheus, no external scrape required.

## 2. Non-goals

- Backward compatibility with the existing `~/.ehco/metrics.db`. Old data is discarded; users may delete the file manually after upgrade.
- Backward compatibility with external Prometheus scrapers consuming `/metrics`. The endpoint is removed.
- Frontend changes. Existing API (`/api/v1/node_metrics`, `/api/v1/rule_metrics`) keeps the same request/response types.
- Migration of historical data.

## 3. Architecture

### Before

```
ehco runtime ──prom Collector.Inc/Add──┐
                                        ▼
                         prometheus Registry (internal/metrics/metrics.go)
                         + node_exporter Collector (Linux only; darwin no-op)
                                        ▼
                         /metrics HTTP endpoint (web/server.go)
                                        ▼
                         self-HTTP scrape via cfg.MetricsURL
                                        ▼
                         pkg/metric_reader (HTTP GET + expfmt parse)
                                        ▼
                         RuleMetrics struct
                                        ▼
                         cmgr.syncOnce → SQLite
                                        ▼
                         /api/v1/node_metrics, /rule_metrics → frontend
```

### After

```
ehco runtime ──metrics.AddBytes/IncConn/RecordHandshake──┐
                                                          ▼
              internal/metrics  (atomic counter store + gopsutil node collector)
                                                          ▼
              metrics.Snapshot()  (in-process function call)
                                                          ▼
              cmgr.syncOnce  →  ms.MetricsStore  →  tstorage
                                                          ▼
              /api/v1/node_metrics, /rule_metrics  →  frontend
```

### Module map

| Module | Action |
|---|---|
| `internal/cmgr/ms/` | **Rewrite.** tstorage-backed; same external interface |
| `internal/metrics/metrics.go` | **Rewrite.** atomic counter store + Snapshot |
| `internal/metrics/node_*.go` | **Replace.** Single cross-platform file using `gopsutil/v4` |
| `internal/metrics/ping.go` | **Keep.** Output to internal counter instead of Prometheus histogram |
| `pkg/metric_reader/` | **Delete entirely.** Replaced by `metrics.Snapshot()` |
| `internal/cmgr/syncer.go` | One-line change: `cm.mr.ReadOnce(ctx)` → `metrics.Snapshot()` |
| `internal/cmgr/cmgr.go` | Drop `mr` field |
| `internal/cmgr/config.go` | Drop `MetricsURL` |
| `internal/web/server.go` | Drop `/metrics` route, drop `RegisterEhcoMetrics` / `RegisterNodeExporterMetrics` calls |
| `internal/relay/server.go` | Drop `MetricsURL` propagation |
| `internal/cli/flags.go` | Drop `--metric-url` flag if present |
| `go.mod` | **+** `nakabonne/tstorage`, `shirou/gopsutil/v4`<br>**−** `modernc.org/sqlite`, `prometheus/client_golang`, `client_model`, `common`, `node_exporter`, possibly `kingpin/v2` |

## 4. Choice rationale

Three candidates were evaluated for the storage engine:

| Option | Verdict |
|---|---|
| `nakabonne/tstorage` | **Chosen.** Pure Go embedded TSDB. Native partitioning + retention + WAL + mmap. API one-to-one with existing schema (metric + labels + ts + value). 1.2k★, last commit 2026-03-16. Maintenance is sparse but interface is small enough to vendor/fork if upstream stalls. |
| `prometheus/prometheus/tsdb` | Pulls the entire prometheus repo dependency tree. Conflicts with the goal of removing Prometheus from the project. |
| `dgraph-io/badger` + custom encoding | Most flexible but 3-5× implementation cost. No upside given tstorage already covers the use case. |

**Risk mitigation for sparse upstream maintenance:** keep `tstorage` import strictly inside `internal/cmgr/ms`. Pin a known-good commit/version in `go.mod`. If upstream breaks: vendor + self-maintain, or fall back to badger + custom encoding.

## 5. Schema

### 5.1 Node-level metrics

Five independent series, no labels.

| tstorage metric | source |
|---|---|
| `node_cpu_usage` | gopsutil cpu percent |
| `node_memory_usage` | gopsutil mem percent |
| `node_disk_usage` | gopsutil disk percent |
| `node_network_in` | gopsutil net io rate (rx bytes/sec) |
| `node_network_out` | gopsutil net io rate (tx bytes/sec) |

### 5.2 Rule-level metrics

Multiple series sliced by `(label, remote, conn_type, flow)`.

| tstorage metric | labels | semantics |
|---|---|---|
| `rule_ping_latency_ms` | `label, remote` | gauge (last sample) |
| `rule_conn_count` | `label, remote, conn_type∈{tcp,udp}` | gauge (active count) |
| `rule_handshake_ms` | `label, remote, conn_type∈{tcp,udp}` | running mean over sync interval (sum/count, reset after snapshot) |
| `rule_bytes_total` | `label, remote, conn_type∈{tcp,udp}, flow∈{tx,rx}` | counter (monotonic since process start) |

`flow=rx` counters are wired now even though no current consumer reads them — frontend rewrite will use them. Cost is negligible: a few extra `atomic.Int64` fields per remote bucket.

### 5.3 Constants

All metric names and label keys/values live in `internal/cmgr/ms/schema.go` to avoid magic strings.

```go
const (
    MetricNodeCPU       = "node_cpu_usage"
    MetricNodeMem       = "node_memory_usage"
    MetricNodeDisk      = "node_disk_usage"
    MetricNodeNetIn     = "node_network_in"
    MetricNodeNetOut    = "node_network_out"

    MetricRulePingMs       = "rule_ping_latency_ms"
    MetricRuleConnCount    = "rule_conn_count"
    MetricRuleHandshakeMs  = "rule_handshake_ms"
    MetricRuleBytesTotal   = "rule_bytes_total"

    LblLabel    = "label"
    LblRemote   = "remote"
    LblConnType = "conn_type"
    LblFlow     = "flow"

    ConnTypeTCP = "tcp"
    ConnTypeUDP = "udp"
    FlowTx      = "tx"
    FlowRx      = "rx"
)
```

### 5.4 Timestamp precision

Seconds (Unix). Configured via `tstorage.WithTimestampPrecision(tstorage.Seconds)`. Sub-second precision is noise at the cmgr sync cadence.

### 5.5 Cardinality estimate

50 rules × 2 remotes × ~9 metrics ≈ 700–900 series. tstorage retains only the active partition in memory; estimated steady-state heap < 50MB.

## 6. Hot-path counter store

### 6.1 Storage layout

```go
type store struct {
    mu    sync.RWMutex
    rules map[string]*ruleBucket // key: rule label

    // node-level snapshots, refreshed by node collector
    nodeCPU atomic.Uint64 // float64 bits via math.Float64bits
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
    tcpConn, udpConn                       atomic.Int64
    tcpBytesTx, tcpBytesRx                 atomic.Int64
    udpBytesTx, udpBytesRx                 atomic.Int64
    tcpHsSum, tcpHsCnt                     atomic.Int64
    udpHsSum, udpHsCnt                     atomic.Int64
    pingLatencyMs                          atomic.Int64
    pingTargetIP                           atomic.Pointer[string]
}
```

### 6.2 Public API

```go
// hot path
func IncConn(label, connType, remote string)
func DecConn(label, connType, remote string)
func AddBytes(label, connType, remote, flow string, n int64)
func RecordHandshake(label, connType, remote string, dur time.Duration)
func RecordPing(label, remote, ip string, latencyMs int64)

// sync path
func Snapshot() (*NodeSnapshot, []*RuleSnapshot)

// lifecycle
func Init(cfg *config.Config) error
func Start(ctx context.Context)
```

### 6.3 Series lookup

Two-level map (label → ruleBucket → remote → remoteBucket) with double-checked locking. Hot-path steady state: 2 RLocks + 2 map lookups + 1 atomic.Add. The first write for a new (label, remote) pair upgrades to write lock; subsequent writes hit the fast path.

### 6.4 Snapshot semantics

- **Counters** (bytes_total): not reset. Downstream is monotonic; frontend can derive rate.
- **Gauges** (conn_count): read current value.
- **Running mean** (handshake_ms): `sum/count`, then `atomic.SwapInt64` both to zero. The next sync interval reports a fresh mean. Result: each datapoint represents the average handshake latency *during that interval* — more useful than process-lifetime mean.
- **Ping** (latencyMs): read current value (overwritten by the ping goroutine each tick).

## 7. Query path

`QueryNodeMetric` and `QueryRuleMetric` external signatures and response types are unchanged.

### 7.1 tstorage query constraints

- `Select(metric, labels, start, end)` — exact label match only, no regex/wildcard.
- No series enumeration API — tstorage cannot list known `(label, remote)` pairs.
- DataPoints returned in ascending timestamp order; `start` inclusive, `end` exclusive.

### 7.2 Series index

Live series index = key set of `metrics.store.rules`. After process restart it starts empty and rebuilds on the next sync tick. Acceptable: no historical (label, remote) is queryable until cmgr writes once, but no historical data exists for that interval anyway.

`metrics` exposes `Pairs(labelFilter, remoteFilter string) []LabelRemote` to feed the query path.

### 7.3 Implementation sketch

```go
func (ms *MetricsStore) QueryNodeMetric(ctx context.Context, req *QueryNodeMetricsReq) (*QueryNodeMetricsResp, error) {
    metricNames := []string{MetricNodeCPU, MetricNodeMem, MetricNodeDisk, MetricNodeNetIn, MetricNodeNetOut}
    // 5 concurrent Select; fan-in by timestamp; sort DESC; truncate to Num
}

func (ms *MetricsStore) QueryRuleMetric(ctx context.Context, req *QueryRuleMetricsReq) (*QueryRuleMetricsResp, error) {
    pairs := ms.idx.Pairs(req.RuleLabel, req.Remote)
    // for each (label, remote): 9 concurrent Select; fan-in by ts; sort DESC; truncate to Num
}
```

`tstorage.ErrNoDataPoints` is treated as empty, not error.

### 7.4 Sort & limit

Preserve current semantics: `ORDER BY ts DESC LIMIT Num`. After fan-in, sort by ts descending and slice to `req.Num`. No server-side downsampling — frontend rewrite may add it later.

## 8. tstorage configuration

| Option | Value |
|---|---|
| `WithDataPath` | `~/.ehco/tsdb/` |
| `WithRetention` | `30 * 24 * time.Hour` |
| `WithPartitionDuration` | tstorage default (1 hour) |
| `WithWALBufferedSize` | tstorage default (4096 bytes) |
| `WithTimestampPrecision` | `tstorage.Seconds` |
| `WithWriteTimeout` | 5s |

### 8.1 Durability boundary

WAL is buffered (4KB). Loss profile:
- **Graceful shutdown** (`Close()`): zero loss.
- **`kill -9` / power loss**: up to ~1–2 seconds of recent samples in the WAL buffer.

Acceptable for monitoring data. Documented for operators.

### 8.2 Disk watermark guard (additional retention layer)

On top of tstorage's built-in 30d retention, a watermark goroutine enforces a disk usage cap.

- Config: `MaxDiskUsagePercent` (default `50`).
- Frequency: every 5 minutes.
- Probe: total used / total size of the filesystem mount containing `~/.ehco/tsdb`.
- On breach: enumerate tstorage partition directories, sort by oldest mtime, delete oldest one at a time, re-probe, repeat until below threshold.
- Logged at warn level on every deletion.

This guards against scenarios where tsdb growth (e.g. a runaway sampling rate, or external disk pressure) would crowd out the rest of the system before 30d retention naturally trims data.

## 9. Lifecycle

### Startup

1. `metrics.Init(cfg)` — initialize counter store
2. `metrics.Start(ctx)` — launch ping goroutine + node collector tick + disk watermark goroutine
3. `ms.NewMetricsStore(dataDir)` — open tstorage at `~/.ehco/tsdb/`
4. Log a warning if legacy `~/.ehco/metrics.db` is found (do not auto-delete).

### Shutdown

`cmgr.Start` defers `ms.Close()` on `ctx.Done()`. `MetricsStore.Close()` calls `tsdb.Close()` which flushes WAL and persists partitions.

## 10. Error handling

| Scenario | Action |
|---|---|
| `NewStorage` fails | Bubble up; ehco fails to start (matches current SQLite behavior) |
| `InsertRows` fails | Log at error; do not block syncOnce; lose this tick's data; retry next tick |
| `Select` fails | Return error to handler; frontend surfaces the error |
| `tstorage.ErrNoDataPoints` | Treat as empty result |
| Data directory corruption | tstorage refuses to open. Manual recovery: `rm -rf ~/.ehco/tsdb` and restart. |

## 11. Testing strategy

- **`internal/metrics`**: concurrent hot-path stress (Add/Inc/Dec from N goroutines, then Snapshot — verify counts, no races, `go test -race`).
- **`internal/cmgr/ms`**: round-trip black-box test — `AddNodeMetric` + `AddRuleMetric` then `QueryNodeMetric` + `QueryRuleMetric`, verify shape and ordering. Time range filtering, label filtering, num cap.
- **`syncer.go`**: integration test — spin up cmgr with a temp tsdb dir, push samples through `metrics.Snapshot()`, verify they land in tstorage.
- **Delete** existing SQLite-specific tests; replace with the above.
- **Disk watermark**: unit test with a fake disk-usage prober; ensure oldest-first deletion order.

## 12. Implementation order

1. `internal/cmgr/ms/` rewrite (tstorage-backed) — pure data layer, isolated, easiest to test.
2. `internal/metrics/` rewrite (atomic store + Snapshot + gopsutil collector + ping output adapter).
3. Wire `cmgr.syncer.go` to `metrics.Snapshot()`. Drop `metric_reader`.
4. Strip `/metrics` HTTP, drop `RegisterEhcoMetrics` / `RegisterNodeExporterMetrics`.
5. Drop `MetricsURL` config + flag plumbing.
6. Disk watermark goroutine.
7. `go.mod` cleanup (`go mod tidy` after dropping prom + sqlite imports).
8. Tests + race-detector run.
9. Smoke test: `go run` locally, hit API endpoints, verify shape.

## 13. Open questions

None. All design decisions finalized; see decision log in conversation log.

## 14. Breaking changes

### Upstream sync JSON shape (`POST <SyncURL>`)

The `node` field of the sync request body changed shape. Old shape (from
`metric_reader.NodeMetrics`):

- `cpu_usage_percent`, `cpu_core_count`, `cpu_load_info`
- `memory_usage_percent`, `memory_total_bytes`, `memory_usage_bytes`
- `disk_usage_percent`, `disk_total_bytes`, `disk_usage_bytes`
- `network_receive_bytes_total`, `network_transmit_bytes_total`,
  `network_receive_bytes_rate`, `network_transmit_bytes_rate`
- `sync_time`

New shape (from `syncNodeMetrics`):

- `timestamp` (Unix seconds)
- `cpu_usage`, `memory_usage`, `disk_usage` (all percent 0-100)
- `network_in`, `network_out` (bytes/sec)

Upstream consumers must update their parsers. Total/raw counters are no longer
sent (only percentages and rates).

### `/metrics` HTTP endpoint removed

External Prometheus scrapers will see 404. Use `/api/v1/node_metrics` and
`/api/v1/rule_metrics` instead, or upgrade your scraper out of band.

### `--metric_url` flag removed

The flag and its `EHCO_METRIC_URL` env var are gone. Remove from any service
unit files / wrapper scripts that set them.
