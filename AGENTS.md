# ehco

ehco is a relay/proxy server combining a custom TCP/WS/WSS relay frontend
and an embedded xray-core for vless / trojan / shadowsocks-2022. Single
static Go binary, configured via JSON file or HTTP endpoint.

## Layout

- `cmd/ehco/main.go` — entry point, defers to `internal/cli`.
- `internal/cli/` — urfave/cli app, flag parsing, boot orchestration in
  `MustStartComponents`.
- `internal/config/` — top-level `Config`, loaded from file or HTTP. The
  same instance is shared across subsystems and reloaded periodically.
- `internal/relay/` — TCP/WS/WSS relay frontend; has its own reloader on
  a ticker (`server_reloader.go`).
- `internal/cmgr/` — connection manager for the relay frontend; tracks
  active/closed conns.
- `internal/web/` — admin HTTP API (echo). Exposes `/metrics/`,
  `/api/v1/...`. Other subsystems mount routes via `webS.APIGroup()`.
- `pkg/xray/` — embedded xray-core. UserPool, connTracker,
  meteredOutbound, admin endpoints. See dedicated section below.

## Build / test

```
make lint        # golangci-lint; must be clean for CI
make test        # full unit suite
make test-e2e    # pkg/xray e2e (~15s, real sockets, runs trojan/vless/ss2022 ± UDP + REALITY)
make build       # static binary
```

For fast iteration: `go test ./pkg/xray/... -count=1`.

CI runs lint + tests on every push; lint failure blocks merge.

## Local dev with real traffic

An idle instance makes the dashboard useless: conns, users, throughput
and the counter registry all read 0. To get real numbers locally, put a
client in front of the trojan inbound and push traffic through it. All of
this is scratch tooling — keep it outside the repo (e.g. `/tmp`).

1. **Config.** `-c <file>` makes the CLI flags inert: every knob
   (`web_port`, `api_token`, `dashboard_pass`, `reload_interval`, the
   inbound's port/password) must live in the file. Two instances at once
   need **separate `$HOME`** — the metrics store is hardcoded to
   `$HOME/.ehco/metrics.db` and SQLite takes an exclusive lock, so the
   second process dies with `database is locked (5)`.

2. **A stand-in upstream.** The user pool — and therefore `users`,
   `running_users` and the traffic totals — only exists when a sync
   endpoint is configured, and the pool pulls the user list from that
   endpoint. Serve a synthetic user over HTTP and point
   `sync_traffic_endpoint` **and** `relay_sync_url` at it. **Never point
   a local instance at a real control plane** — it would report traffic
   into production.

   ```
   GET  /  -> {"users":[{"user_id":9001,"password":"demopass","enable":true,"protocol":"trojan"}]}
   POST /  -> 200 {}
   ```

3. **Client.** `sing-box` works as a trojan client. `socks` for curl, a
   `direct` (dokodemo) inbound so a plain TCP client can be tunneled:

   ```json
   { "inbounds": [
       { "type": "socks",  "tag": "socks-in", "listen": "127.0.0.1", "listen_port": 1080 },
       { "type": "direct", "tag": "iperf-in", "listen": "127.0.0.1", "listen_port": 5301,
         "override_address": "127.0.0.1", "override_port": 5302 } ],
     "outbounds": [
       { "type": "trojan", "tag": "trojan-out", "server": "127.0.0.1", "server_port": 4443,
         "password": "demopass", "tls": { "enabled": true, "insecure": true } } ],
     "route": { "rules": [ { "action": "route", "outbound": "trojan-out" } ], "final": "trojan-out" } }
   ```

   `tls.insecure` is required unless the config ships its own cert —
   ehco injects a self-signed one.

4. **Traffic.**
   - Throughput: `iperf3 -s -p 5302`, then `iperf3 -c 127.0.0.1 -p 5301`
     (the dokodemo inbound forwards to 5302 *through* the proxy).
   - Connection churn (`conn_total`):
     `for i in $(seq 1 10); do curl -s --socks5-hostname 127.0.0.1:1080 -o /dev/null http://127.0.0.1:8099/; done`

5. **Two things that bite.**
   - Host metrics (the Home charts) are sampled only when
     `relay_sync_url` is set — see `NeedStartCmgr` in
     `internal/config/config.go`. A node with a web server but no relay
     sync creates the SQLite store and never writes to it. Point it at
     the stand-in upstream to get charts.
   - The per-cycle traffic counters reset every sync tick; the
     cumulative-since-boot numbers are on `/api/v1/xray/users`.

## Boot order is load-bearing

`MustStartComponents` in `internal/cli/config.go` starts subsystems in
this exact order:

1. relay server (goroutine)
2. webS = `web.NewServer(...)` (constructed, not yet listening)
3. `webS.Start()` (goroutine — must come before xray)
4. `xrayS.Setup()` → `RegisterRoutes(webS.APIGroup())` → `Start()`

xray's `UserPool` runs its first sync **synchronously** inside
`xrayS.Start`, and that sync GETs the local `/metrics/` endpoint for
bandwidth recording. If web isn't listening yet, the fetch fails. We
tolerate it (warn + 0 bandwidth + retry next tick), but the order still
matters — don't reorder without a reason.

Echo accepts route registration after `Start`, so registering xray's
routes via `APIGroup()` after `webS.Start()` is fine.

## Config gotcha: shared `*Config` + xray-conf UnmarshalJSON

`*config.Config` is a single instance shared by relay's reloader and
xray's reloader. Both call `LoadConfig` periodically. xray-conf has
types whose `UnmarshalJSON` **appends** rather than replaces (notably
`PortList.Range`). Re-decoding into a stale struct accumulates state,
which made xray's `needReload` listener comparison spuriously fire
("old has 2 ranges, new has 1"), and every spurious reload kills all
active conns via `tracker.KillAll`.

`LoadConfig` therefore nils out decoded sub-structs (`c.RelayConfigs =
nil; c.XRayConfig = nil`) before re-unmarshaling. **If you add a new
top-level field with a non-trivial UnmarshalJSON, reset it here too.**

## xray/ package architecture

We embed xray-core (`v1.260206.0` at time of writing) in-process and
**bypass its gRPC control plane**:

- **User CRUD**: instead of `HandlerService.AlterInbound` over gRPC,
  call `inbound.Manager.GetHandler(tag).(proxy.UserManager).AddUser/
  RemoveUser` directly. xray's gRPC commander is just a wrapper around
  this same interface, so we save the loopback round-trip.
- **Traffic stats**: instead of `StatsService.QueryStats`, the
  `meteredOutbound` (replaces freedom as the default outbound) wraps
  the dialed conn's `buf.Reader/Writer` and bumps atomic counters on
  `*User` per chunk. Atomic swap-and-reset on each sync tick.
- **Conn tracking**: `connTracker` registers each Dispatch entry,
  holding `*session.Inbound` + `*session.Outbound` pointers directly
  (no field duplication). Powers `/api/v1/xray/conns` admin endpoints
  for list/kill — xray's native `RemoveUserOperation` only blocks new
  conns and won't kick existing ones.

### `stripUnused`

`server.go::stripUnused` removes `cfg.API/Stats/Policy/OutboundConfigs`
and the api-tagged inbound from the parsed xray config before
`core.New`, so xray falls back to `policy.DefaultManager` and
`stats.NoopManager`. Don't re-introduce these without a reason — they
bind ports and accumulate counters we don't read.

### User identity

xray's `protocol.User.Email` carries the **decimal-string user_id** by
convention (set by upstream when posting user configs). Use
`userIDFromInbound(inb)` to parse. **Don't put real emails there**;
nothing else in the system handles them.

### Reload kills all conns

When `needReload` detects a listener change, `Reload` calls `Stop`
which calls `tracker.KillAll()`. This is by design — port changed,
can't keep serving the old listener. So a spurious `needReload` drops
every active user. Make any change to `needReload` carefully, and
prefer comparing structured state (port slices, listen addr) over
proto string-formatting which is mutation-sensitive.

### Per-cycle traffic reporting

`syncTrafficToServer` runs every `SyncTime` seconds (default 60).
Each cycle, for each user:

- `UploadTraffic / DownloadTraffic` — `atomic.SwapInt64` to 0 on snapshot.
- `IPList` — `mergeLiveIPs(snapshotted user.recentIPs,
  tracker.List(userID))`. The merge is essential: `RecordIP` only
  fires once per Dispatch (conn open), so long-lived conns spanning
  multiple cycles would otherwise show empty IPs after their first
  cycle even while traffic flows.
- `TcpCount` — `tracker.CountTCPByUser(userID)`, instantaneous live
  count at snapshot time.

`recentIPs` is FIFO with cap `maxRecentIPsPerUser` (10); overflow logs
a warning and drops the oldest.

Bandwidth fetch failure is **non-fatal**: warn + report 0, don't drop
the user traffic upload. If POST itself fails after retries, the
snapshotted batch is **lost** (TODO in code — local replay buffer
would be the right fix). Don't add code paths that snapshot+reset
without handling this.

### `common.Interrupt` errcheck

`xray-core/common.Interrupt(reader_or_writer)` returns an error.
Always discard with `_ = common.Interrupt(...)` — lint will fail
otherwise. The call is best-effort cleanup; xray-core itself ignores
the return.

## Logging

zap, named per subsystem (`zap.L().Named("xray")`, `Named("user_pool")`,
etc.). Sugar is fine for human-readable lines. Important diagnostic
output (e.g. the `syncTrafficToServer payload: ...` line) goes through
`Sugar().Infof` so it shows up at the default log level.

If you change a log line's prefix or wording, future debugging may
break — leave them stable unless you have a reason.

## Code style

- English only in code, comments, identifiers, commit messages.
  Conversation with the user can be Chinese.
- Tests live alongside code (`foo.go` → `foo_test.go`).
- Don't comment on *what* the code does. Reserve comments for *why* —
  non-obvious constraints, historical incidents, semantics that aren't
  visible from naming.
- Match xray-core's idioms when interacting with it (e.g. `*session.X`
  pointers held by value, `protocol.MemoryUser` construction). Don't
  invent abstractions over xray types where direct use is clearer.

## Commit / PR conventions

- Branch names: `xray/...`, `feat/...`, `fix/...`, `chore/...`.
- Commit subjects: `<area>: <imperative summary>`, lowercase prefix.
  Examples in `git log`: `xray: ...`, `fix: ...`, `feat(cli): ...`.
- Open PRs with `gh pr create`. Everything that lands in the repo —
  PR titles **and** bodies, commit messages, comments, docs — is
  English. Only the conversation with the user may be Chinese.
