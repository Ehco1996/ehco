package cmgr

var DummyConfig = &Config{}

type Config struct {
	SyncURL      string `json:"sync_url,omitempty"`
	SyncDuration int    `json:"sync_duration"` // in seconds
}

func (c *Config) NeedSync() bool {
	return c.SyncURL != ""
}

func (c *Config) Adjust() {
	if c.SyncDuration <= 0 {
		c.SyncDuration = 60
	}
}
