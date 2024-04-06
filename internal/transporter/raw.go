// nolint: errcheck
package transporter

import (
	"net"
	"time"

	"go.uber.org/zap"

	"github.com/Ehco1996/ehco/internal/cmgr"
	"github.com/Ehco1996/ehco/internal/conn"
	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/metrics"
	"github.com/Ehco1996/ehco/pkg/lb"
)

type Raw struct {
	relayLabel string

	// TCP
	cmgr       cmgr.Cmgr
	tCPRemotes lb.RoundRobin

	l *zap.SugaredLogger
}

func newRaw(relayLabel string, tcpRemotes lb.RoundRobin, cmgr cmgr.Cmgr) *Raw {
	r := &Raw{
		cmgr:       cmgr,
		relayLabel: relayLabel,
		tCPRemotes: tcpRemotes,
		l:          zap.S().Named(relayLabel),
	}
	return r
}

func (raw *Raw) GetRemote() *lb.Node {
	return raw.tCPRemotes.Next()
}

func (raw *Raw) dialRemote(remote *lb.Node) (net.Conn, error) {
	t1 := time.Now()
	d := net.Dialer{Timeout: constant.DialTimeOut}
	rc, err := d.Dial("tcp", remote.Address)
	if err != nil {
		return nil, err
	}
	latency := time.Since(t1)
	metrics.HandShakeDuration.WithLabelValues(remote.Label).Observe(float64(latency.Milliseconds()))
	remote.HandShakeDuration = latency
	return rc, nil
}

func (raw *Raw) HandleTCPConn(c net.Conn, remote *lb.Node) error {
	metrics.CurConnectionCount.WithLabelValues(remote.Label, metrics.METRIC_CONN_TYPE_TCP).Inc()
	defer metrics.CurConnectionCount.WithLabelValues(remote.Label, metrics.METRIC_CONN_TYPE_TCP).Dec()

	clonedRemote := remote.Clone()
	rc, err := raw.dialRemote(clonedRemote)
	if err != nil {
		return err
	}
	raw.l.Infof("HandleTCPConn from %s to %s", c.LocalAddr(), remote.Address)
	relayConn := conn.NewRelayConn(raw.relayLabel, c, rc, conn.WithHandshakeDuration(clonedRemote.HandShakeDuration))
	raw.cmgr.AddConnection(relayConn)
	defer raw.cmgr.RemoveConnection(relayConn)
	return relayConn.Transport(remote.Label)
}
