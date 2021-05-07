package lb

import (
	"sync/atomic"
)

// RoundRobin is an interface for representing round-robin balancing.
type RoundRobin interface {
	Next() string
}

type roundrobin struct {
	remotes []string
	next    uint32
}

func NewRBRemotes(remotes []string) RoundRobin {
	return &roundrobin{remotes: remotes}
}

func (r *roundrobin) Next() string {
	n := atomic.AddUint32(&r.next, 1)
	return r.remotes[(int(n)-1)%len(r.remotes)]
}
