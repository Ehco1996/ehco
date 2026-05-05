package cmgr

type Config struct {
	SyncURL      string
	SyncInterval int // in seconds

	// EnableMetrics opens the local SQLite metrics store and starts the
	// host / rule samplers. Off when there is no web server to surface
	// the data — sampling without a reader is just disk churn.
	EnableMetrics bool
}

func (c *Config) NeedSync() bool {
	return c.SyncURL != "" && c.SyncInterval > 0
}

func (c *Config) NeedMetrics() bool {
	return c.EnableMetrics && c.SyncInterval > 0
}

func (c *Config) Adjust() {
	if c.SyncInterval <= 0 {
		c.SyncInterval = 60
	}
}
