package lb

import (
	"github.com/Ehco1996/ehco/pkg/log"
	"go.uber.org/atomic"
)

type Node struct {
	Address string
	Label   string

	BlockTimes *atomic.Int64
}

func (n *Node) BlockForSomeTime() {
	// TODO: make this configurable
	n.BlockTimes.Add(1000)
	log.InfoLogger.Infof("[lb] block remote node for 1000 times label=%s remote=%s", n.Label, n.Address)
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
	if next.BlockTimes.Load() > 0 {
		next.BlockTimes.Dec()
		return r.Next()
	}
	return next
}
