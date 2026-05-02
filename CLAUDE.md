# CLAUDE.md

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
- Open PRs with `gh pr create`. The conversation language is fine in
  the PR body, but keep the title in English.
