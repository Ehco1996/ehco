package glue

import (
	"context"
)

type Reloader interface {
	Reload(force bool) error
}

type HealthChecker interface {
	// get relay by ID and check the connection health
	HealthCheck(ctx context.Context, RelayID string) (int64, error)
}

// XrayStatus is the slice of XrayServer the web admin needs for its
// aggregate /overview endpoint. Defined here so web/ doesn't need to
// import pkg/xray.
type XrayStatus interface {
	// Snapshot returns instantaneous counters scraped from the user
	// pool and conn tracker. Cheap — no DB hits.
	Snapshot() XraySnapshot
}

type XraySnapshot struct {
	Conns         int   `json:"conns"`
	Users         int   `json:"users"`
	EnabledUsers  int   `json:"enabled_users"`
	RunningUsers  int   `json:"running_users"`
	UploadTotal   int64 `json:"upload_total"`
	DownloadTotal int64 `json:"download_total"`
}
