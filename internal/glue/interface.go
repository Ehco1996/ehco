package glue

import (
	"context"
	"time"
)

type Reloader interface {
	Reload(force bool) error
}

type HealthChecker interface {
	// get relay by ID and check the connection health
	HealthCheck(ctx context.Context, RelayID string) (int64, error)
}

// XrayStatus is the slice of XrayServer the web admin exposes. Defined
// here so web/ doesn't need to import pkg/xray.
type XrayStatus interface {
	// Snapshot returns instantaneous counters scraped from the user
	// pool and conn tracker. Cheap — no DB hits.
	Snapshot() XraySnapshot
	// RunningInbounds maps proxy inbound tag -> "listen,port" for the
	// listeners xray is currently on. Drifted reports whether the
	// newest config still differs from it (a reload is pending or has
	// failed).
	RunningInbounds() map[string]string
	Drifted() bool
	// ConfigSync / TrafficSync are the outcomes of the most recent
	// upstream round-trips: fetching the config, and reporting traffic.
	ConfigSync() SyncStatus
	TrafficSync() SyncStatus
	// RecentEvents is the in-memory lifecycle log, oldest first.
	RecentEvents() []RuntimeEvent
	// Counters is the flat since-start counter registry (HAProxy
	// `show stat` / nginx stub_status shape): name -> value.
	Counters() map[string]int64
}

type XraySnapshot struct {
	Conns         int   `json:"conns"`
	Users         int   `json:"users"`
	EnabledUsers  int   `json:"enabled_users"`
	RunningUsers  int   `json:"running_users"`
	UploadTotal   int64 `json:"upload_total"`
	DownloadTotal int64 `json:"download_total"`
}

// SyncStatus is the outcome of the most recent sync with the upstream
// control plane. Zero value means it never ran.
type SyncStatus struct {
	OK    bool      `json:"ok"`
	At    time.Time `json:"at,omitzero"`
	Error string    `json:"error,omitempty"`
}

// RuntimeEvent is one entry in the node's lifecycle log: config fetch
// errors, config drift, reload attempts. Deliberately in-memory and not
// persisted — once the process restarts, its events describe a dead
// process, so a clean slate is the honest view.
type RuntimeEvent struct {
	At     time.Time `json:"at"`
	Kind   string    `json:"kind"`
	Detail string    `json:"detail,omitempty"`
}
