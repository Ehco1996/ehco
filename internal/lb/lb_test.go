package lb

import (
	"fmt"
	"testing"
)

func TestLBNodeHeap(t *testing.T) {
	nodes := map[string]int{
		"1.1.1.1": 1, "4.4.4.4": 4, "3.3.3.3": 3,
	}

	lp := make(LBNodes, len(nodes))
	i := 0
	for value, priority := range nodes {
		lp[i] = &LBNode{
			Remote:        value,
			OnLineUserCnt: priority,
			index:         i,
		}
		i++
	}
	lp.HeapInit()
	// Insert a new item and then modify its priority.
	node := &LBNode{
		Remote:        "0.0.0.0",
		OnLineUserCnt: 0,
	}
	// heap.Push(&lp, node)
	lp.HeapPush(node)
	if lp.MinLBNode() != node {
		t.Fatalf("MinLBNode: %v != node: %v", lp.MinLBNode(), node)
	}
	// Take the items out; they arrive in decreasing priority order.
	for lp.Len() > 0 {
		node := lp.HeapPop()
		fmt.Printf("%d : %s \n", node.OnLineUserCnt, node.Remote)
	}
}
