package xray

import (
	"context"
	"net"
	"strconv"
	"testing"

	xnet "github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/session"
)

// fakeConn is a minimal net.Conn that just tracks Close calls.
type fakeConn struct {
	net.Conn
	closed bool
}

func (c *fakeConn) Close() error { c.closed = true; return nil }

func newFakeConn() *fakeConn { return &fakeConn{} }

// makeSessions builds synthetic inbound/outbound session structs for tests.
// email is also used as user_id (matches the production convention where the
// email field carries the numeric id as a string).
func makeSessions(email, srcIP, target string, network xnet.Network) (*session.Inbound, *session.Outbound) {
	inb := &session.Inbound{
		User:   &protocol.MemoryUser{Email: email},
		Source: xnet.Destination{Network: network, Address: xnet.ParseAddress(srcIP), Port: 12345},
	}
	ob := &session.Outbound{
		Target: xnet.Destination{Network: network, Address: xnet.ParseAddress(target), Port: 443},
	}
	return inb, ob
}

func TestConnTrackerRegisterAndUnregister(t *testing.T) {
	tr := newConnTracker()
	_, cancel := context.WithCancel(context.Background())
	inb, ob := makeSessions("1", "10.0.0.1", "1.1.1.1", xnet.Network_TCP)
	id := tr.Register(inb, ob, newFakeConn(), cancel)
	if id == 0 {
		t.Fatal("expected non-zero id")
	}
	if got := tr.List(0); len(got) != 1 {
		t.Fatalf("List(0) want 1, got %d", len(got))
	}
	if got := tr.List(1); len(got) != 1 {
		t.Fatalf("List(1) want 1, got %d", len(got))
	}
	// derived fields should be readable through ConnInfo
	info := tr.List(1)[0]
	if info.UserID != 1 || info.Email != "1" || info.SourceIP != "10.0.0.1" || info.Network != "tcp" {
		t.Fatalf("unexpected info: %+v", info)
	}
	tr.Unregister(id)
	if got := tr.List(0); len(got) != 0 {
		t.Fatalf("after unregister want 0, got %d", len(got))
	}
	// byUser bucket should be cleaned up
	tr.mu.RLock()
	_, ok := tr.byUser[1]
	tr.mu.RUnlock()
	if ok {
		t.Fatal("byUser bucket leaked after unregister")
	}
}

func TestConnTrackerKill(t *testing.T) {
	tr := newConnTracker()
	ctx, cancel := context.WithCancel(context.Background())
	conn := newFakeConn()
	inb, ob := makeSessions("7", "10.0.0.7", "x", xnet.Network_TCP)
	id := tr.Register(inb, ob, conn, cancel)
	if !tr.Kill(id) {
		t.Fatal("Kill should return true for known id")
	}
	if !conn.closed {
		t.Fatal("conn.Close not invoked")
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("ctx not cancelled")
	}
	// Kill leaves the entry registered; Unregister is the outbound's
	// responsibility once buf.Copy returns. After Unregister, Kill should
	// no-op.
	tr.Unregister(id)
	if tr.Kill(id) {
		t.Fatal("Kill should return false after Unregister")
	}
}

func TestConnTrackerKillByUser(t *testing.T) {
	tr := newConnTracker()
	for i := 0; i < 3; i++ {
		_, cancel := context.WithCancel(context.Background())
		inb, ob := makeSessions("42", "10.0.0.42", "x", xnet.Network_TCP)
		tr.Register(inb, ob, newFakeConn(), cancel)
	}
	_, cancel := context.WithCancel(context.Background())
	inb, ob := makeSessions("99", "10.0.0.99", "x", xnet.Network_TCP)
	tr.Register(inb, ob, newFakeConn(), cancel)

	if n := tr.KillByUser(42); n != 3 {
		t.Fatalf("KillByUser(42) want 3, got %d", n)
	}
	// other user's conn should remain registered
	if got := tr.List(99); len(got) != 1 {
		t.Fatalf("user 99 should still have 1 conn, got %d", len(got))
	}
}

func TestConnTrackerKillAll(t *testing.T) {
	tr := newConnTracker()
	for u := 1; u <= 4; u++ {
		_, cancel := context.WithCancel(context.Background())
		inb, ob := makeSessions(strconv.Itoa(u), "10.0.0.1", "x", xnet.Network_TCP)
		tr.Register(inb, ob, newFakeConn(), cancel)
	}
	if n := tr.KillAll(); n != 4 {
		t.Fatalf("KillAll want 4, got %d", n)
	}
}

func TestConnTrackerCountTCPByUser(t *testing.T) {
	tr := newConnTracker()
	_, c1 := context.WithCancel(context.Background())
	inbT, obT := makeSessions("5", "10.0.0.5", "x", xnet.Network_TCP)
	tr.Register(inbT, obT, newFakeConn(), c1)

	_, c2 := context.WithCancel(context.Background())
	inbU, obU := makeSessions("5", "10.0.0.5", "x", xnet.Network_UDP)
	tr.Register(inbU, obU, newFakeConn(), c2)

	if n := tr.CountTCPByUser(5); n != 1 {
		t.Fatalf("CountTCPByUser(5) want 1 (only the TCP conn), got %d", n)
	}
}
