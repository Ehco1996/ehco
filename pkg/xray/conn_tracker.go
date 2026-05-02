package xray

import (
	"context"
	"net"
	"sync"
	"time"
)

type connEntry struct {
	id      uint64
	userID  int
	email   string
	network string
	target  string
	since   time.Time
	cancel  context.CancelFunc
	conn    net.Conn
}

type ConnInfo struct {
	ID      uint64    `json:"id"`
	UserID  int       `json:"user_id"`
	Email   string    `json:"email"`
	Network string    `json:"network"`
	Target  string    `json:"target"`
	Since   time.Time `json:"since"`
}

func (e *connEntry) info() ConnInfo {
	return ConnInfo{
		ID:      e.id,
		UserID:  e.userID,
		Email:   e.email,
		Network: e.network,
		Target:  e.target,
		Since:   e.since,
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

func (t *connTracker) Register(userID int, email, network, target string, conn net.Conn, cancel context.CancelFunc) uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.nextID++
	id := t.nextID
	e := &connEntry{
		id:      id,
		userID:  userID,
		email:   email,
		network: network,
		target:  target,
		since:   time.Now(),
		cancel:  cancel,
		conn:    conn,
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

func killEntry(e *connEntry) {
	if e.cancel != nil {
		e.cancel()
	}
	if e.conn != nil {
		_ = e.conn.Close()
	}
}
