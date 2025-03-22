package cmgr

type Config struct {
	SyncURL      string
	MetricsURL   string
	SyncInterval int // in seconds
}

func (c *Config) NeedSync() bool {
	return c.SyncURL != "" && c.SyncInterval > 0
}

func (c *Config) NeedMetrics() bool {
	return c.MetricsURL != "" && c.SyncInterval > 0
}

func (c *Config) Adjust() {
	if c.SyncInterval <= 0 {
		c.SyncInterval = 60
	}
}
