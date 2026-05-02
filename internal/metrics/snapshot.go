package metrics

import (
	"math"
	"sync/atomic"
	"time"

	"github.com/Ehco1996/ehco/internal/cmgr/ms"
)

func Snapshot() (*ms.NodeSnapshot, []*ms.RuleSnapshot) {
	now := time.Now()
	node := &ms.NodeSnapshot{
		SyncTime:    now,
		CPUUsage:    math.Float64frombits(globalStore.nodeCPU.Load()),
		MemoryUsage: math.Float64frombits(globalStore.nodeMem.Load()),
		DiskUsage:   math.Float64frombits(globalStore.nodeDsk.Load()),
		NetworkIn:   math.Float64frombits(globalStore.nodeIn.Load()),
		NetworkOut:  math.Float64frombits(globalStore.nodeOut.Load()),
	}

	globalStore.mu.RLock()
	rules := make([]*ms.RuleSnapshot, 0, len(globalStore.rules))
	for _, rb := range globalStore.rules {
		rules = append(rules, snapshotRule(rb, now))
	}
	globalStore.mu.RUnlock()
	return node, rules
}

func snapshotRule(rb *ruleBucket, now time.Time) *ms.RuleSnapshot {
	rb.mu.RLock()
	remotes := make([]ms.RemoteSnapshot, 0, len(rb.remotes))
	for remote, b := range rb.remotes {
		remotes = append(remotes, ms.RemoteSnapshot{
			Remote:         remote,
			PingLatencyMs:  b.pingLatencyMs.Load(),
			TCPConnCount:   b.tcpConn.Load(),
			UDPConnCount:   b.udpConn.Load(),
			TCPHandshakeMs: drainMean(&b.tcpHsSum, &b.tcpHsCnt),
			UDPHandshakeMs: drainMean(&b.udpHsSum, &b.udpHsCnt),
			TCPBytesTx:     b.tcpBytesTx.Load(),
			TCPBytesRx:     b.tcpBytesRx.Load(),
			UDPBytesTx:     b.udpBytesTx.Load(),
			UDPBytesRx:     b.udpBytesRx.Load(),
		})
	}
	rb.mu.RUnlock()
	return &ms.RuleSnapshot{SyncTime: now, Label: rb.label, Remotes: remotes}
}

func drainMean(sum, cnt *atomic.Int64) int64 {
	s := sum.Swap(0)
	c := cnt.Swap(0)
	if c == 0 {
		return 0
	}
	return s / c
}

// Pairs implements ms.PairLister.
type Pairs struct{}

func (Pairs) Pairs(labelFilter, remoteFilter string) []ms.LabelRemote {
	live := globalStore.listPairs(labelFilter, remoteFilter)
	out := make([]ms.LabelRemote, len(live))
	for i, lr := range live {
		out[i] = ms.LabelRemote{Label: lr.Label, Remote: lr.Remote}
	}
	return out
}
