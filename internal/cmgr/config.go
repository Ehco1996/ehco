package cmgr

var (
	DummyConfig = &Config{}
)

type Config struct {
	SyncURL      string `json:"sync_url,omitempty"`
	SyncDuration int    `json:"sync_duration"` // in seconds
}

func (c *Config) NeedSync() bool {
	return c.SyncURL != "" && c.SyncDuration > 0
}
