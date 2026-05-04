package ms

import (
	"sync/atomic"
	"time"
)

// opStats accumulates count + total + max + last latency for one named
// SQL operation. Lifetime-since-process-start semantics; reset via
// (*Stats).Reset to clear a one-off spike.
type opStats struct {
	count   atomic.Int64
	totalNs atomic.Int64
	maxNs   atomic.Int64
	lastNs  atomic.Int64
}

func (s *opStats) record(d time.Duration) {
	ns := d.Nanoseconds()
	s.count.Add(1)
	s.totalNs.Add(ns)
	s.lastNs.Store(ns)
	for {
		m := s.maxNs.Load()
		if ns <= m || s.maxNs.CompareAndSwap(m, ns) {
			return
		}
	}
}

func (s *opStats) reset() {
	s.count.Store(0)
	s.totalNs.Store(0)
	s.maxNs.Store(0)
	s.lastNs.Store(0)
}

// OpStatsSnapshot is the serialisable view of one opStats. Durations
// are reported in milliseconds — the SPA never needs sub-ms precision.
type OpStatsSnapshot struct {
	Count  int64   `json:"count"`
	AvgMs  float64 `json:"avg_ms"`
	MaxMs  float64 `json:"max_ms"`
	LastMs float64 `json:"last_ms"`
}

func (s *opStats) snapshot() OpStatsSnapshot {
	count := s.count.Load()
	out := OpStatsSnapshot{
		Count:  count,
		MaxMs:  float64(s.maxNs.Load()) / 1e6,
		LastMs: float64(s.lastNs.Load()) / 1e6,
	}
	if count > 0 {
		out.AvgMs = float64(s.totalNs.Load()) / float64(count) / 1e6
	}
	return out
}

// Stats bundles every tracked op. Add a field here, register it in
// (*Stats).All, and the dashboard picks it up automatically.
type Stats struct {
	AddNode    opStats
	AddRule    opStats
	QueryNode  opStats
	QueryRule  opStats
	Cleanup    opStats
	Vacuum     opStats
	Truncate   opStats
}

func (s *Stats) all() []namedOp {
	return []namedOp{
		{"add_node", &s.AddNode},
		{"add_rule", &s.AddRule},
		{"query_node", &s.QueryNode},
		{"query_rule", &s.QueryRule},
		{"cleanup", &s.Cleanup},
		{"vacuum", &s.Vacuum},
		{"truncate", &s.Truncate},
	}
}

type namedOp struct {
	name string
	s    *opStats
}

// Snapshot renders every tracked op into a name-keyed map suitable
// for direct JSON encoding.
func (s *Stats) Snapshot() map[string]OpStatsSnapshot {
	all := s.all()
	out := make(map[string]OpStatsSnapshot, len(all))
	for _, n := range all {
		out[n.name] = n.s.snapshot()
	}
	return out
}

// Reset clears all tracked ops in one call. Used by the
// "Reset stats" Settings button.
func (s *Stats) Reset() {
	for _, n := range s.all() {
		n.s.reset()
	}
}

// track is the canonical instrumentation helper. Idiomatic use:
//
//	defer track(&ms.stats.QueryRule)()
//
// The returned closure captures the start time at the point of
// invocation, so the deferred call records (now - start) regardless of
// how the function unwinds.
func track(s *opStats) func() {
	start := time.Now()
	return func() { s.record(time.Since(start)) }
}
