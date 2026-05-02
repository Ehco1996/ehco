package cmgr

type Config struct {
	SyncInterval        int
	SyncURL             string
	MaxDiskUsagePercent int // 0 = use default (50)
}

func (c *Config) NeedSync() bool {
	return c.SyncURL != ""
}

func (c *Config) NeedMetrics() bool {
	return c.SyncInterval > 0
}

func (c *Config) Adjust() {
	if c.SyncInterval <= 0 {
		c.SyncInterval = 60
	}
}
