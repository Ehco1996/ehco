package lb

import (
	"container/heap"
)

type LBNode struct {
	Remote        string //value
	OnLineUserCnt int    //priority
	index         int
}

type LBNodeHeap []*LBNode

// update modifies the priority and value of an Item in the queue.
func (lp *LBNodeHeap) update(node *LBNode, remote string, cnt int) {
	node.Remote = remote
	node.OnLineUserCnt = cnt
	heap.Fix(lp, node.index)
}

func (lp LBNodeHeap) Len() int { return len(lp) }

func (lp LBNodeHeap) Less(i, j int) bool {
	return lp[i].OnLineUserCnt < lp[j].OnLineUserCnt
}

func (lp LBNodeHeap) Swap(i, j int) {
	lp[i], lp[j] = lp[j], lp[i]
	lp[i].index = i
	lp[j].index = j
}

func (lp *LBNodeHeap) Push(x interface{}) {
	n := len(*lp)
	node := x.(*LBNode)
	node.index = n
	*lp = append(*lp, node)
}

func (lp *LBNodeHeap) Pop() interface{} {
	old := *lp
	n := len(old)
	node := old[n-1]
	old[n-1] = nil  // avoid memory leak
	node.index = -1 // for safety
	*lp = old[0 : n-1]
	return node
}

func (lp *LBNodeHeap) HeapInit() {
	heap.Init(lp)
}

func (lp *LBNodeHeap) HeapPush(node *LBNode) {
	heap.Push(lp, node)
}

func (lp *LBNodeHeap) HeapPop() *LBNode {
	return heap.Pop(lp).(*LBNode)
}

func (lp *LBNodeHeap) MinLBNode() *LBNode {
	if lp.Len() > 0 {
		old := *lp
		return old[0]
	}
	return nil
}
