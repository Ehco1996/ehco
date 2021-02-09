package lb

import (
	"container/heap"
)

type LBNode struct {
	Remote        string //value
	OnLineUserCnt int    //priority
	index         int
}

type LBNodes []*LBNode

func New(remotes []string) *LBNodes {
	lh := make(LBNodes, len(remotes))
	for i, remote := range remotes {
		lh[i] = &LBNode{
			Remote:        remote,
			OnLineUserCnt: 0,
			index:         i,
		}
	}
	lh.HeapInit()
	return &lh
}

func (lp *LBNodes) Len() int { return len(*lp) }

func (lp *LBNodes) Less(i, j int) bool {
	l := *lp
	return l[i].OnLineUserCnt < l[j].OnLineUserCnt
}

func (lp *LBNodes) Swap(i, j int) {
	l := *lp
	l[i], l[j] = l[j], l[i]
	l[i].index = i
	l[j].index = j
}

func (lp *LBNodes) Push(x interface{}) {
	n := len(*lp)
	node := x.(*LBNode)
	node.index = n
	*lp = append(*lp, node)
}

func (lp *LBNodes) Pop() interface{} {
	old := *lp
	n := len(old)
	node := old[n-1]
	old[n-1] = nil  // avoid memory leak
	node.index = -1 // for safety
	*lp = old[0 : n-1]
	return node
}

// update modifies the priority and value of an Item in the queue.
func (lp *LBNodes) update(node *LBNode, remote string, cnt int) {
	node.Remote = remote
	node.OnLineUserCnt = cnt
	heap.Fix(lp, node.index)
}

func (lp *LBNodes) HeapInit() {
	heap.Init(lp)
}

func (lp *LBNodes) HeapPush(node *LBNode) {
	heap.Push(lp, node)
}

func (lp *LBNodes) HeapPop() *LBNode {
	return heap.Pop(lp).(*LBNode)
}

func (lp *LBNodes) MinLBNode() *LBNode {
	if lp.Len() > 0 {
		old := *lp
		return old[0]
	}
	return nil
}

func (lp *LBNodes) IncrUserCnt(node *LBNode, num int) {
	lp.update(node, node.Remote, node.OnLineUserCnt+num)
}

func (lp *LBNodes) PickMin() *LBNode {
	node := lp.MinLBNode()
	lp.IncrUserCnt(node, 1)
	return node
}

func (lp *LBNodes) DeferPick(node *LBNode) {
	lp.IncrUserCnt(node, -1)
}
