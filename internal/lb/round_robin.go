package lb

import (
	"sync/atomic"
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
	next     uint32

	len int
}

func NewRoundRobin(nodeList []*Node) RoundRobin {
	len := len(nodeList)
	return &roundrobin{nodeList: nodeList, len: len}
}

func (r *roundrobin) Next() *Node {
	n := atomic.AddUint32(&r.next, 1)
	return r.nodeList[(int(n)-1)%r.len]
}
