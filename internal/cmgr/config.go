package cmgr

var DummyConfig = &Config{}

type Config struct {
	SyncURL      string `json:"sync_url,omitempty"`
	SyncInterval int    `json:"sync_interval,omitempty"` // in seconds
}

func (c *Config) NeedSync() bool {
	return c.SyncURL != ""
}

func (c *Config) Adjust() {
	if c.SyncInterval <= 0 {
		c.SyncInterval = 60
	}
}
