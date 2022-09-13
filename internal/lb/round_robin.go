package lb

import (
	"go.uber.org/atomic"
)

type Node struct {
	Address string
	Label   string
}

// RoundRobin is an interface for representing round-robin balancing.
type RoundRobin interface {
	Next() *Node
}

type roundrobin struct {
	nodeList []*Node
	next     *atomic.Int64

	len int
}

func NewRoundRobin(nodeList []*Node) RoundRobin {
	len := len(nodeList)
	next := atomic.NewInt64(0)
	return &roundrobin{nodeList: nodeList, len: len, next: next}
}

func (r *roundrobin) Next() *Node {
	n := r.next.Add(1)
	next := r.nodeList[(int(n)-1)%r.len]
	return next
}
