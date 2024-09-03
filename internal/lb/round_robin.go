package lb

import (
	"net/url"
	"strings"
	"time"

	"go.uber.org/atomic"
)

type Node struct {
	Address           string
	HandShakeDuration time.Duration
}

func (n *Node) Clone() *Node {
	return &Node{
		Address:           n.Address,
		HandShakeDuration: n.HandShakeDuration,
	}
}

func extractHost(input string) (string, error) {
	// Check if the input string has a scheme, if not, add "http://"
	if !strings.Contains(input, "://") {
		input = "http://" + input
	}
	// Parse the URL
	u, err := url.Parse(input)
	if err != nil {
		return "", err
	}
	return u.Hostname(), nil
}

// NOTE for (https/ws/wss)://xxx.com -> xxx.com
func (n *Node) GetAddrHost() (string, error) {
	return extractHost(n.Address)
}

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
	n := r.next.Add(1)
	next := r.nodeList[(int(n)-1)%r.len]
	return next
}

func (r *roundrobin) GetAll() []*Node {
	return r.nodeList
}
