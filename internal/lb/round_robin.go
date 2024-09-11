package lb

import (
	"go.uber.org/atomic"
)

// RoundRobin is an interface for representing round-robin balancing.
type RoundRobin interface {
	Next() *Remote
	GetAll() []*Remote
}

type roundrobin struct {
	nodeList []*Remote
	next     *atomic.Int64
	len      int
}

func NewRoundRobin(nodeList []*Remote) RoundRobin {
	len := len(nodeList)
	next := atomic.NewInt64(0)
	return &roundrobin{nodeList: nodeList, len: len, next: next}
}

func (r *roundrobin) Next() *Remote {
	if r.len == 0 {
		return nil
	}
	n := r.next.Add(1)
	next := r.nodeList[(int(n)-1)%r.len]
	return next
}

func (r *roundrobin) GetAll() []*Remote {
	return r.nodeList
}
