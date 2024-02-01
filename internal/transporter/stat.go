package transporter

type ConnStats interface {
	// RecordTraffic records the traffic of the connection
	RecordTraffic(down, up int64)

	ReSetTraffic()
}

func NewConnStats() ConnStats {
	return &connStatsImpl{}
}

type connStatsImpl struct {
	down int64
	up   int64
}

func (c *connStatsImpl) RecordTraffic(down, up int64) {
	c.down += down
	c.up += up
}

func (c *connStatsImpl) ReSetTraffic() {
	c.down = 0
	c.up = 0
}
