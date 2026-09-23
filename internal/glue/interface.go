package glue

import "context"

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
}

type XraySnapshot struct {
	Conns         int   `json:"conns"`
	Users         int   `json:"users"`
	EnabledUsers  int   `json:"enabled_users"`
	RunningUsers  int   `json:"running_users"`
	UploadTotal   int64 `json:"upload_total"`
	DownloadTotal int64 `json:"download_total"`
}
