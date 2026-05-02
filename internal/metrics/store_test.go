package metrics

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHotPath_ConcurrentWriters(t *testing.T) {
	globalStore = newStore()
	const writers = 50
	const writesPer = 1000

	var wg sync.WaitGroup
	wg.Add(writers)
	for i := 0; i < writers; i++ {
		go func(i int) {
			defer wg.Done()
			label := "rule" + string(rune('A'+i%5))
			remote := "10.0.0." + string(rune('1'+i%9))
			for j := 0; j < writesPer; j++ {
				IncConn(label, ConnTypeTCP, remote)
				AddBytes(label, ConnTypeTCP, remote, FlowTx, 100)
				AddBytes(label, ConnTypeTCP, remote, FlowRx, 50)
				RecordHandshake(label, ConnTypeTCP, remote, 5*time.Millisecond)
				DecConn(label, ConnTypeTCP, remote)
			}
		}(i)
	}
	wg.Wait()

	_, rules := Snapshot()

	var totalConn, totalTx, totalRx int64
	for _, rs := range rules {
		for _, b := range rs.Remotes {
			totalConn += b.TCPConnCount
			totalTx += b.TCPBytesTx
			totalRx += b.TCPBytesRx
		}
	}
	require.Equal(t, int64(0), totalConn) // Inc/Dec cancel
	require.Equal(t, int64(writers*writesPer*100), totalTx)
	require.Equal(t, int64(writers*writesPer*50), totalRx)
}

func TestSnapshot_DrainsHandshakeMean(t *testing.T) {
	globalStore = newStore()
	RecordHandshake("r", ConnTypeTCP, "x", 10*time.Millisecond)
	RecordHandshake("r", ConnTypeTCP, "x", 20*time.Millisecond)

	_, rules := Snapshot()
	require.Len(t, rules, 1)
	require.Equal(t, int64(15), rules[0].Remotes[0].TCPHandshakeMs)

	_, rules = Snapshot()
	require.Equal(t, int64(0), rules[0].Remotes[0].TCPHandshakeMs)
}

func TestPairs_Filtering(t *testing.T) {
	globalStore = newStore()
	IncConn("a", ConnTypeTCP, "r1")
	IncConn("a", ConnTypeTCP, "r2")
	IncConn("b", ConnTypeTCP, "r1")

	require.Len(t, Pairs{}.Pairs("", ""), 3)
	require.Len(t, Pairs{}.Pairs("a", ""), 2)
	require.Len(t, Pairs{}.Pairs("", "r1"), 2)
	require.Len(t, Pairs{}.Pairs("a", "r1"), 1)
}
