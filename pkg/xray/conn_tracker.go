package xray

import (
	"context"
	"net"
	"strconv"
	"sync"
	"time"

	xnet "github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/session"
)

// connEntry holds a thin wrapper around xray's per-Dispatch session pointers
// plus our own bookkeeping (id/since/cancel/conn). Source/User/Target are read
// directly off the session structs, so we don't duplicate fields and pick up
// any future xray additions for free.
type connEntry struct {
	id     uint64
	userID int // cached from inbound.User.Email atoi
	since  time.Time
	cancel context.CancelFunc
	conn   net.Conn // outbound dialed conn (Kill closes this)

	inbound  *session.Inbound
	outbound *session.Outbound
}

func (e *connEntry) email() string {
	if e.inbound == nil || e.inbound.User == nil {
		return ""
	}
	return e.inbound.User.Email
}

func (e *connEntry) sourceIP() string {
	return sourceIPFromInbound(e.inbound)
}

func (e *connEntry) target() string {
	if e.outbound == nil {
		return ""
	}
	return e.outbound.Target.NetAddr()
}

func (e *connEntry) network() string {
	if e.outbound == nil {
		return ""
	}
	if e.outbound.Target.Network == xnet.Network_UDP {
		return "udp"
	}
	return "tcp"
}

// sourceIPFromInbound returns the client-side address as a string, or "" if
// the inbound session doesn't carry one. Returns the literal Address.String()
// (so callers see the raw form — IPv4, IPv6, or, in pathological cases, a
// domain).
func sourceIPFromInbound(inb *session.Inbound) string {
	if inb == nil || inb.Source.Address == nil {
		return ""
	}
	return inb.Source.Address.String()
}

// userIDFromInbound parses the email field as an int. Returns 0 when the
// inbound has no user or the email isn't numeric.
func userIDFromInbound(inb *session.Inbound) int {
	if inb == nil || inb.User == nil {
		return 0
	}
	id, err := strconv.Atoi(inb.User.Email)
	if err != nil {
		return 0
	}
	return id
}

type ConnInfo struct {
	ID       uint64    `json:"id"`
	UserID   int       `json:"user_id"`
	Email    string    `json:"email"`
	Network  string    `json:"network"`
	Target   string    `json:"target"`
	SourceIP string    `json:"source_ip"`
	Since    time.Time `json:"since"`
}

func (e *connEntry) info() ConnInfo {
	return ConnInfo{
		ID:       e.id,
		UserID:   e.userID,
		Email:    e.email(),
		Network:  e.network(),
		Target:   e.target(),
		SourceIP: e.sourceIP(),
		Since:    e.since,
	}
}

type connTracker struct {
	mu      sync.RWMutex
	nextID  uint64
	entries map[uint64]*connEntry
	byUser  map[int]map[uint64]struct{}
}

func newConnTracker() *connTracker {
	return &connTracker{
		entries: make(map[uint64]*connEntry),
		byUser:  make(map[int]map[uint64]struct{}),
	}
}

func (t *connTracker) Register(inbound *session.Inbound, outbound *session.Outbound, conn net.Conn, cancel context.CancelFunc) uint64 {
	userID := userIDFromInbound(inbound)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.nextID++
	id := t.nextID
	e := &connEntry{
		id:       id,
		userID:   userID,
		since:    time.Now(),
		cancel:   cancel,
		conn:     conn,
		inbound:  inbound,
		outbound: outbound,
	}
	t.entries[id] = e
	bucket, ok := t.byUser[userID]
	if !ok {
		bucket = make(map[uint64]struct{})
		t.byUser[userID] = bucket
	}
	bucket[id] = struct{}{}
	return id
}

func (t *connTracker) Unregister(id uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.entries[id]
	if !ok {
		return
	}
	delete(t.entries, id)
	if bucket, ok := t.byUser[e.userID]; ok {
		delete(bucket, id)
		if len(bucket) == 0 {
			delete(t.byUser, e.userID)
		}
	}
}

// Kill cancels the conn's ctx and closes the underlying conn. Returns false if id not found.
func (t *connTracker) Kill(id uint64) bool {
	t.mu.RLock()
	e, ok := t.entries[id]
	t.mu.RUnlock()
	if !ok {
		return false
	}
	killEntry(e)
	return true
}

func (t *connTracker) KillByUser(userID int) int {
	t.mu.RLock()
	bucket := t.byUser[userID]
	victims := make([]*connEntry, 0, len(bucket))
	for id := range bucket {
		if e, ok := t.entries[id]; ok {
			victims = append(victims, e)
		}
	}
	t.mu.RUnlock()
	for _, e := range victims {
		killEntry(e)
	}
	return len(victims)
}

func (t *connTracker) KillAll() int {
	t.mu.RLock()
	victims := make([]*connEntry, 0, len(t.entries))
	for _, e := range t.entries {
		victims = append(victims, e)
	}
	t.mu.RUnlock()
	for _, e := range victims {
		killEntry(e)
	}
	return len(victims)
}

// List returns a snapshot. userID == 0 means all users.
func (t *connTracker) List(userID int) []ConnInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if userID == 0 {
		out := make([]ConnInfo, 0, len(t.entries))
		for _, e := range t.entries {
			out = append(out, e.info())
		}
		return out
	}
	bucket := t.byUser[userID]
	out := make([]ConnInfo, 0, len(bucket))
	for id := range bucket {
		if e, ok := t.entries[id]; ok {
			out = append(out, e.info())
		}
	}
	return out
}

// Count returns the total number of live conns across all users.
func (t *connTracker) Count() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.entries)
}

// CountTCPByUser returns how many live TCP conns the user currently has.
// Used by the traffic sync to populate UserTraffic.TcpCount.
func (t *connTracker) CountTCPByUser(userID int) int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	bucket := t.byUser[userID]
	n := 0
	for id := range bucket {
		if e, ok := t.entries[id]; ok && e.network() == "tcp" {
			n++
		}
	}
	return n
}

func killEntry(e *connEntry) {
	if e.cancel != nil {
		e.cancel()
	}
	if e.conn != nil {
		_ = e.conn.Close()
	}
}
