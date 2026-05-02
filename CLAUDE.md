# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`ehco` is a lightweight TCP/UDP relay and tunneling tool written in Go. A single binary forwards traffic between hosts using a few transport types (`raw`, `ws`, `wss`), optionally exposing a small web UI and an HTTP sync hook for centralized stats collection.

## Common commands

All standard workflows go through the Makefile.

```bash
make build              # CGO_ENABLED=0 binary into dist/ehco
make test               # go test -v -count=1 -timeout=1m ./...
make lint               # golangci-lint via tools/bin
make fmt                # golangci-lint --fix + gofumpt
make tidy               # go mod tidy
make tools              # bootstrap golangci-lint and gofumpt into tools/bin
```

Run a single test: `go test -tags ${BUILD_TAG_FOR_NODE_EXPORTER} -v -run TestX ./internal/foo/...`. On Linux you must include the build tags `nofibrechannel,nomountstats` (the Makefile injects them automatically; replicate manually when calling `go test`/`go build` directly). `CGO_ENABLED=0` is non-negotiable — the project intentionally stays pure Go.

Local end-to-end runs use the configs in `localdev/` (e.g. `dist/ehco -c localdev/node-1.json`). `examples/` contains documentation-style sample configs; `test/bench/` is the iperf-based throughput rig.

## Top-level shape

```
cmd/ehco/main.go          → cli.CreateCliAPP()
internal/cli/             → urfave/cli/v2 app, flags, install + update subcommands,
                            InitConfigAndComponents / MustStartComponents bootstrap
internal/config/          → top-level config schema (loaded from file or HTTP URL)
internal/relay/           → Relay lifecycle; relay/conf/ holds per-rule config
internal/transporter/     → raw / ws / wss listeners + clients (the actual byte plumbing)
internal/conn/            → RelayConn, where bytes accounting happens
internal/cmgr/            → connection manager, sync ticker, embedded metrics store
internal/cmgr/ms/         → time-series store backing /api/v1/{node,rule}_metrics
internal/metrics/         → in-process counters + node info collector + ICMP ping group
internal/web/             → echo HTTP server: API, web UI templates, optional /metrics
pkg/metric_reader/        → expfmt parser that scrapes the in-process /metrics
pkg/xray/                 → optional xray-core integration for traffic accounting
```

## Architectural pattern: the metrics pipeline

This is the most non-obvious part of the codebase, so understand it before touching `internal/metrics`, `internal/cmgr`, `internal/web`, or `pkg/metric_reader`.

Today (current `master`):

1. Runtime code (transporter/conn) calls `prometheus.CounterVec.Inc()` / `HistogramVec.Observe()` against package-level collectors in `internal/metrics`.
2. `internal/metrics` registers those collectors plus `prometheus/node_exporter`'s `NodeCollector` (Linux only — darwin's `node_darwin.go` is a no-op stub).
3. `internal/web/server.go` exposes `/metrics` via `promhttp.Handler()`.
4. `pkg/metric_reader/reader.go` HTTP-scrapes that same endpoint inside the same process and parses the exposition text via `expfmt`.
5. `internal/cmgr/syncer.go` calls `metric_reader.ReadOnce`, writes the parsed snapshot into `internal/cmgr/ms` (SQLite via `modernc.org/sqlite`), and optionally POSTs to `cfg.SyncURL`.
6. `pkg/xray/bandwidth_recorder.go` separately scrapes the same `/metrics` endpoint for `node_network_*_bytes_total` to feed xray traffic accounting.

The HTTP self-scrape is a load-bearing oddity: it's the only path by which counters in step 1 reach the SQLite store in step 5.

**Active refactor in progress.** This pipeline is being replaced. See:
- `docs/superpowers/specs/2026-05-02-tsdb-replacement-design.md` — design
- `docs/superpowers/plans/2026-05-02-tsdb-replacement.md` — task-by-task plan

The destination state is: atomic counter store inside `internal/metrics`, gopsutil-based node info, `Snapshot()` direct call instead of HTTP scrape, `nakabonne/tstorage` replacing SQLite, all `prometheus/*` and `node_exporter` deps removed, `/metrics` endpoint deleted. If asked to add a new metric or change one, check whether the refactor has landed first (`grep prometheus go.mod`); if it has, follow the new `metrics.IncConn / AddBytes / RecordHandshake / RecordPing` API and stay out of the deleted Prometheus surface.

## Architectural pattern: relay assembly

`cli.MustStartComponents` builds three layers from one config:

- **`cmgr.Cmgr`** — single instance, owns the metrics store and the sync ticker (`SyncInterval`), exposed query methods serve `/api/v1/*`.
- **`relay.Relay` per rule** — each rule from `cfg.RelayConfigs` becomes a `Relay`, which wraps a `transporter.RelayServer`.
- **`transporter.RelayServer`** — concrete listener (`raw`/`ws`/`wss`) + a `RelayClient` for the upstream side. The base server in `transporter/base.go` owns connection counting and rate limiting; subclasses customize the I/O loop.

`internal/relay/server_reloader.go` watches the config (file or HTTP) and rebuilds the relay set when it changes. New rules become new `Relay`s; removed rules are stopped. The reloader interval is `--config_reload_interval` (default 60s).

## Conventions worth preserving

- **Pure Go.** No CGO. The Makefile pins `CGO_ENABLED=0`. SQLite is `modernc.org/sqlite`, not `mattn/go-sqlite3`. Adding a CGO dep needs a deliberate decision.
- **Two CLI frameworks coexist by accident.** `urfave/cli/v2` is the actual app surface; `alecthomas/kingpin/v2` is pulled in only because `prometheus/node_exporter` calls `kingpin.CommandLine.Parse([]string{})` as a side-effect to seed default collectors (`internal/metrics/node_linux.go:41`). Don't use kingpin for new flags. (After the refactor lands, kingpin should be removable entirely.)
- **Persistent state lives under `~/.ehco/`.** Currently `metrics.db` (SQLite); after the refactor `tsdb/` (tstorage data dir).
- **`internal/constant`** holds build-time `-ldflags` injected vars (`Version`, `GitRevision`, `GitBranch`, `BuildTime`) — these are populated by the Makefile, not by code.
