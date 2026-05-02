package metrics

import "time"

const (
	ConnTypeTCP = "tcp"
	ConnTypeUDP = "udp"
	FlowTx      = "tx"
	FlowRx      = "rx"
)

func IncConn(label, connType, remote string) {
	rb := globalStore.getOrCreateRemote(label, remote)
	switch connType {
	case ConnTypeTCP:
		rb.tcpConn.Add(1)
	case ConnTypeUDP:
		rb.udpConn.Add(1)
	}
}

func DecConn(label, connType, remote string) {
	rb := globalStore.getOrCreateRemote(label, remote)
	switch connType {
	case ConnTypeTCP:
		rb.tcpConn.Add(-1)
	case ConnTypeUDP:
		rb.udpConn.Add(-1)
	}
}

func AddBytes(label, connType, remote, flow string, n int64) {
	rb := globalStore.getOrCreateRemote(label, remote)
	switch connType {
	case ConnTypeTCP:
		if flow == FlowTx {
			rb.tcpBytesTx.Add(n)
		} else {
			rb.tcpBytesRx.Add(n)
		}
	case ConnTypeUDP:
		if flow == FlowTx {
			rb.udpBytesTx.Add(n)
		} else {
			rb.udpBytesRx.Add(n)
		}
	}
}

func RecordHandshake(label, connType, remote string, dur time.Duration) {
	rb := globalStore.getOrCreateRemote(label, remote)
	ms := dur.Milliseconds()
	switch connType {
	case ConnTypeTCP:
		rb.tcpHsSum.Add(ms)
		rb.tcpHsCnt.Add(1)
	case ConnTypeUDP:
		rb.udpHsSum.Add(ms)
		rb.udpHsCnt.Add(1)
	}
}

func RecordPing(label, remote, ip string, latencyMs int64) {
	rb := globalStore.getOrCreateRemote(label, remote)
	rb.pingLatencyMs.Store(latencyMs)
	ipCopy := ip
	rb.pingTargetIP.Store(&ipCopy)
}
