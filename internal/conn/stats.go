package conn

import (
	"fmt"
	"net"

	"github.com/Ehco1996/ehco/internal/web"
	"github.com/Ehco1996/ehco/pkg/bytes"
)

type Stats struct {
	up   int64
	down int64
}

func (s *Stats) String() string {
	return fmt.Sprintf("up: %s, down: %s", bytes.PrettyByteSize(float64(s.up)), bytes.PrettyByteSize(float64(s.down)))
}

func (s *Stats) Record(up, down int64) {
	s.up += up
	s.down += down
}

type metricsConn struct {
	net.Conn

	underlyingConn net.Conn
	remoteLabel    string
	stats          *Stats
}

func (c metricsConn) Read(p []byte) (n int, err error) {
	n, err = c.underlyingConn.Read(p)
	// increment the metric for the read bytes
	web.NetWorkTransmitBytes.WithLabelValues(
		c.remoteLabel, web.METRIC_CONN_TYPE_TCP, web.METRIC_CONN_FLOW_READ,
	).Add(float64(n))
	// record the traffic
	c.stats.Record(int64(n), 0)
	return
}

func (c metricsConn) Write(p []byte) (n int, err error) {
	n, err = c.underlyingConn.Write(p)
	web.NetWorkTransmitBytes.WithLabelValues(
		c.remoteLabel, web.METRIC_CONN_TYPE_TCP, web.METRIC_CONN_FLOW_WRITE,
	).Add(float64(n))
	c.stats.Record(0, int64(n))
	return
}
