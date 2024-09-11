package lb

import (
	"go.uber.org/atomic"
)

// RoundRobin is an interface for representing round-robin balancing.
type RoundRobin interface {
	Next() *Node
	GetAll() []*Node
}

type roundrobin struct {
	nodeList []*Node
	next     *atomic.Int64
	len      int
}

func NewRoundRobin(nodeList []*Node) RoundRobin {
	len := len(nodeList)
	next := atomic.NewInt64(0)
	return &roundrobin{nodeList: nodeList, len: len, next: next}
}

func (r *roundrobin) Next() *Node {
	if r.len == 0 {
		return nil
	}
	n := r.next.Add(1)
	next := r.nodeList[(int(n)-1)%r.len]
	return next
}

func (r *roundrobin) GetAll() []*Node {
	return r.nodeList
}
