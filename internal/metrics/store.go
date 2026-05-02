package metrics

import (
	"math"
	"sort"
	"sync"
	"sync/atomic"
)

var globalStore = newStore()

type store struct {
	mu    sync.RWMutex
	rules map[string]*ruleBucket

	nodeCPU atomic.Uint64 // float64 bits
	nodeMem atomic.Uint64
	nodeDsk atomic.Uint64
	nodeIn  atomic.Uint64
	nodeOut atomic.Uint64
}

type ruleBucket struct {
	label   string
	mu      sync.RWMutex
	remotes map[string]*remoteBucket
}

type remoteBucket struct {
	tcpConn atomic.Int64
	udpConn atomic.Int64

	tcpBytesTx atomic.Int64
	tcpBytesRx atomic.Int64
	udpBytesTx atomic.Int64
	udpBytesRx atomic.Int64

	tcpHsSum atomic.Int64
	tcpHsCnt atomic.Int64
	udpHsSum atomic.Int64
	udpHsCnt atomic.Int64

	pingLatencyMs atomic.Int64
	pingTargetIP  atomic.Pointer[string]
}

func newStore() *store { return &store{rules: make(map[string]*ruleBucket)} }

func (s *store) getOrCreateRemote(label, remote string) *remoteBucket {
	s.mu.RLock()
	rb := s.rules[label]
	s.mu.RUnlock()
	if rb == nil {
		s.mu.Lock()
		if rb = s.rules[label]; rb == nil {
			rb = &ruleBucket{label: label, remotes: make(map[string]*remoteBucket)}
			s.rules[label] = rb
		}
		s.mu.Unlock()
	}

	rb.mu.RLock()
	rem := rb.remotes[remote]
	rb.mu.RUnlock()
	if rem == nil {
		rb.mu.Lock()
		if rem = rb.remotes[remote]; rem == nil {
			rem = &remoteBucket{}
			rb.remotes[remote] = rem
		}
		rb.mu.Unlock()
	}
	return rem
}

func (s *store) setNode(cpu, mem, disk, netIn, netOut float64) {
	s.nodeCPU.Store(math.Float64bits(cpu))
	s.nodeMem.Store(math.Float64bits(mem))
	s.nodeDsk.Store(math.Float64bits(disk))
	s.nodeIn.Store(math.Float64bits(netIn))
	s.nodeOut.Store(math.Float64bits(netOut))
}

type labelRemote struct {
	Label  string
	Remote string
}

func (s *store) listPairs(labelFilter, remoteFilter string) []labelRemote {
	s.mu.RLock()
	rules := make([]*ruleBucket, 0, len(s.rules))
	for _, rb := range s.rules {
		if labelFilter != "" && rb.label != labelFilter {
			continue
		}
		rules = append(rules, rb)
	}
	s.mu.RUnlock()

	out := make([]labelRemote, 0, len(rules))
	for _, rb := range rules {
		rb.mu.RLock()
		for remote := range rb.remotes {
			if remoteFilter != "" && remote != remoteFilter {
				continue
			}
			out = append(out, labelRemote{Label: rb.label, Remote: remote})
		}
		rb.mu.RUnlock()
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Label != out[j].Label {
			return out[i].Label < out[j].Label
		}
		return out[i].Remote < out[j].Remote
	})
	return out
}
