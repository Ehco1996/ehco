package cmgr

import (
	"testing"
	"time"

	"github.com/Ehco1996/ehco/internal/conn"
)

type mockConn struct {
	label string
	stats conn.Stats
}

func (m *mockConn) Transport() error       { return nil }
func (m *mockConn) GetRelayLabel() string  { return m.label }
func (m *mockConn) GetStats() *conn.Stats { return &m.stats }
func (m *mockConn) Close() error           { return nil }

func TestCmgr_ActiveAndDrain(t *testing.T) {
	mgr := NewCmgr(true)

	c1 := &mockConn{label: "rule-1", stats: conn.Stats{Up: 100, Down: 200, HandShakeLatency: 10 * time.Millisecond}}
	c2 := &mockConn{label: "rule-1", stats: conn.Stats{Up: 300, Down: 400, HandShakeLatency: 20 * time.Millisecond}}
	c3 := &mockConn{label: "rule-2", stats: conn.Stats{Up: 50, Down: 50, HandShakeLatency: 5 * time.Millisecond}}

	mgr.AddConnection(c1)
	mgr.AddConnection(c2)
	mgr.AddConnection(c3)

	if cnt := mgr.CountConnection(ConnectionTypeActive); cnt != 3 {
		t.Fatalf("expected 3 active conns, got %d", cnt)
	}
	if cnt := mgr.GetActiveConnectCntByRelayLabel("rule-1"); cnt != 2 {
		t.Fatalf("expected 2 active for rule-1, got %d", cnt)
	}

	mgr.RemoveConnection(c1)
	if cnt := mgr.CountConnection(ConnectionTypeActive); cnt != 2 {
		t.Fatalf("expected 2 active conns after remove, got %d", cnt)
	}
	if cnt := mgr.CountConnection(ConnectionTypeClosed); cnt != 1 {
		t.Fatalf("expected 1 closed conn, got %d", cnt)
	}

	drained := mgr.DrainClosedConnections()
	if len(drained["rule-1"]) != 1 {
		t.Fatalf("expected 1 drained conn for rule-1, got %d", len(drained["rule-1"]))
	}
	if cnt := mgr.CountConnection(ConnectionTypeClosed); cnt != 0 {
		t.Fatalf("expected 0 closed conns after drain, got %d", cnt)
	}
}

func TestCmgr_NoTrackClosed(t *testing.T) {
	mgr := NewCmgr(false)

	c1 := &mockConn{label: "rule-1"}
	mgr.AddConnection(c1)
	mgr.RemoveConnection(c1)

	if cnt := mgr.CountConnection(ConnectionTypeClosed); cnt != 0 {
		t.Fatalf("expected 0 closed conns when trackClosed is false, got %d", cnt)
	}
	drained := mgr.DrainClosedConnections()
	if len(drained) != 0 {
		t.Fatalf("expected empty drained map when trackClosed is false")
	}
}
