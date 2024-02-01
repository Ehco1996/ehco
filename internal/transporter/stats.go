package transporter

import (
	"fmt"
	"math"
)

func PrettyByteSize(bf float64) string {
	for _, unit := range []string{"", "Ki", "Mi", "Gi", "Ti", "Pi", "Ei", "Zi"} {
		if math.Abs(bf) < 1024.0 {
			return fmt.Sprintf(" %3.1f%sB ", bf, unit)
		}
		bf /= 1024.0
	}
	return fmt.Sprintf(" %.1fYiB ", bf)
}

type Stats struct {
	up   int64
	down int64
}

func (s *Stats) ReSet() {
	s.up = 0
	s.down = 0
}

func (s *Stats) String() string {
	return fmt.Sprintf("up: %s, down: %s", PrettyByteSize(float64(s.up)), PrettyByteSize(float64(s.down)))
}

type ConnStats interface {
	RecordTraffic(down, up int64)

	ReSetTraffic()

	GetStats() *Stats
}

func NewConnStats() ConnStats {
	return &connStatsImpl{s: &Stats{up: 0, down: 0}}
}

type connStatsImpl struct {
	s *Stats
}

func (c *connStatsImpl) RecordTraffic(down, up int64) {
	c.s.down += down
	c.s.up += up
}

func (c *connStatsImpl) ReSetTraffic() {
	c.s.ReSet()
}

func (c *connStatsImpl) GetStats() *Stats {
	return c.s
}
