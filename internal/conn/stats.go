package conn

import (
	"fmt"
	"net"

	"github.com/Ehco1996/ehco/internal/metrics"
	"github.com/Ehco1996/ehco/pkg/bytes"
)

type Stats struct {
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

func (s *Stats) Record(up, down int64) {
	s.Up += up
	s.Down += down
}

func (s *Stats) String() string {
	return fmt.Sprintf("up: %s, down: %s", bytes.PrettyByteSize(float64(s.Up)), bytes.PrettyByteSize(float64(s.Down)))
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
	metrics.NetWorkTransmitBytes.WithLabelValues(
		c.remoteLabel, metrics.METRIC_CONN_TYPE_TCP, metrics.METRIC_CONN_FLOW_READ,
	).Add(float64(n))
	// record the traffic
	c.stats.Record(int64(n), 0)
	return
}

func (c metricsConn) Write(p []byte) (n int, err error) {
	n, err = c.underlyingConn.Write(p)
	metrics.NetWorkTransmitBytes.WithLabelValues(
		c.remoteLabel, metrics.METRIC_CONN_TYPE_TCP, metrics.METRIC_CONN_FLOW_WRITE,
	).Add(float64(n))
	c.stats.Record(0, int64(n))
	return
}
