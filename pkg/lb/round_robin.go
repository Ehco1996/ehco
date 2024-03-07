package lb

import (
	"time"

	"go.uber.org/atomic"
)

// todo: move to internal/lb
type Node struct {
	Address           string
	Label             string
	HandShakeDuration time.Duration
}

func (n *Node) Clone() *Node {
	return &Node{
		Address:           n.Address,
		Label:             n.Label,
		HandShakeDuration: n.HandShakeDuration,
	}
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
