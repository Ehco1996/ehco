package xray

import (
	"sync"
	"time"

	"github.com/Ehco1996/ehco/internal/glue"
)

// maxEvents caps the lifecycle ring — a few hours at the normal reload
// cadence, and it keeps a failure loop from growing without bound.
const maxEvents = 20

// syncStatus records the outcome of the most recent upstream sync so a
// stuck control plane shows up on /overview instead of only in the logs.
type syncStatus struct {
	mu sync.Mutex
	st glue.SyncStatus
}

func (s *syncStatus) record(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.st = glue.SyncStatus{OK: err == nil, At: time.Now()}
	if err != nil {
		s.st.Error = err.Error()
	}
}

func (s *syncStatus) get() glue.SyncStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.st
}

// eventRing is a bounded, in-memory log of this process's lifecycle.
type eventRing struct {
	mu  sync.Mutex
	buf []glue.RuntimeEvent
}

func (r *eventRing) add(kind, detail string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf = append(r.buf, glue.RuntimeEvent{At: time.Now(), Kind: kind, Detail: detail})
	if len(r.buf) > maxEvents {
		r.buf = r.buf[len(r.buf)-maxEvents:]
	}
}

func (r *eventRing) list() []glue.RuntimeEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]glue.RuntimeEvent, len(r.buf))
	copy(out, r.buf)
	return out
}
