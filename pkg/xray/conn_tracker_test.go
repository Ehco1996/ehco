package xray

import (
	"context"
	"net"
	"testing"
)

// fakeConn is a minimal net.Conn that just tracks Close calls.
type fakeConn struct {
	net.Conn
	closed bool
}

func (c *fakeConn) Close() error { c.closed = true; return nil }

func newFakeConn() *fakeConn { return &fakeConn{} }

func TestConnTrackerRegisterAndUnregister(t *testing.T) {
	tr := newConnTracker()
	_, cancel := context.WithCancel(context.Background())
	id := tr.Register(1, "1", "tcp", "1.1.1.1:443", newFakeConn(), cancel)
	if id == 0 {
		t.Fatal("expected non-zero id")
	}
	if got := tr.List(0); len(got) != 1 {
		t.Fatalf("List(0) want 1, got %d", len(got))
	}
	if got := tr.List(1); len(got) != 1 {
		t.Fatalf("List(1) want 1, got %d", len(got))
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
	id := tr.Register(7, "7", "tcp", "x:1", conn, cancel)
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
		tr.Register(42, "42", "tcp", "x:1", newFakeConn(), cancel)
	}
	_, cancel := context.WithCancel(context.Background())
	tr.Register(99, "99", "tcp", "x:1", newFakeConn(), cancel)

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
		tr.Register(u, "", "tcp", "x:1", newFakeConn(), cancel)
	}
	if n := tr.KillAll(); n != 4 {
		t.Fatalf("KillAll want 4, got %d", n)
	}
}
