package lb

import (
	"testing"

	"go.uber.org/atomic"
)

func Test_roundrobin_Next(t *testing.T) {
	remotes := []string{
		"127.0.0.1",
		"127.0.0.2",
	}
	nodeList := make([]*Node, len(remotes))
	for i := range remotes {
		nodeList[i] = &Node{Address: remotes[i], BlockTimes: atomic.NewInt64(0)}
	}
	rb := NewRoundRobin(nodeList)

	// normal round robin, should return node one by one
	for i := 0; i < len(remotes); i++ {
		if node := rb.Next(); node.Address != remotes[i] {
			t.Fatalf("need %s got %s", remotes[i], node.Address)
		}
	}

	// block node 0 twice
	node0 := nodeList[0]
	node0.BlockTimes.Add(2)
	// we should get node 1 twice
	for i := 0; i < 2; i++ {
		if node := rb.Next(); node.Address == node0.Address {
			t.Fatal("must not get node0")
		}
	}
	// now node 0 is not blocked, normal round robin again
	for i := 0; i < len(remotes); i++ {
		if node := rb.Next(); node.Address != remotes[i] {
			t.Fatalf("need %s got %s", remotes[i], node.Address)
		}
	}
}
