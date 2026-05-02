package ms

import "time"

// Query request/response (frontend contract — preserved from old SQLite store).

type NodeMetrics struct {
	Timestamp   int64   `json:"timestamp"`
	CPUUsage    float64 `json:"cpu_usage"`
	MemoryUsage float64 `json:"memory_usage"`
	DiskUsage   float64 `json:"disk_usage"`
	NetworkIn   float64 `json:"network_in"`
	NetworkOut  float64 `json:"network_out"`
}

type QueryNodeMetricsReq struct {
	StartTimestamp int64
	EndTimestamp   int64
	Num            int64
}

type QueryNodeMetricsResp struct {
	TOTAL int           `json:"total"`
	Data  []NodeMetrics `json:"data"`
}

type RuleMetricsData struct {
	Timestamp               int64  `json:"timestamp"`
	Label                   string `json:"label"`
	Remote                  string `json:"remote"`
	PingLatency             int64  `json:"ping_latency"`
	TCPConnectionCount      int64  `json:"tcp_connection_count"`
	TCPHandshakeDuration    int64  `json:"tcp_handshake_duration"`
	TCPNetworkTransmitBytes int64  `json:"tcp_network_transmit_bytes"`
	UDPConnectionCount      int64  `json:"udp_connection_count"`
	UDPHandshakeDuration    int64  `json:"udp_handshake_duration"`
	UDPNetworkTransmitBytes int64  `json:"udp_network_transmit_bytes"`
}

type QueryRuleMetricsReq struct {
	RuleLabel      string
	Remote         string
	StartTimestamp int64
	EndTimestamp   int64
	Num            int64
}

type QueryRuleMetricsResp struct {
	TOTAL int               `json:"total"`
	Data  []RuleMetricsData `json:"data"`
}

// Snapshot inputs (from internal/metrics).

type NodeSnapshot struct {
	SyncTime    time.Time
	CPUUsage    float64
	MemoryUsage float64
	DiskUsage   float64
	NetworkIn   float64
	NetworkOut  float64
}

type RemoteSnapshot struct {
	Remote string

	PingLatencyMs int64

	TCPConnCount int64
	UDPConnCount int64

	// Mean over snapshot interval (zero if no new handshakes).
	TCPHandshakeMs int64
	UDPHandshakeMs int64

	// Counters (monotonic since process start).
	TCPBytesTx int64
	TCPBytesRx int64
	UDPBytesTx int64
	UDPBytesRx int64
}

type RuleSnapshot struct {
	SyncTime time.Time
	Label    string
	Remotes  []RemoteSnapshot
}

// PairLister discovers known (label, remote) pairs (live index from internal/metrics).
type PairLister interface {
	Pairs(labelFilter, remoteFilter string) []LabelRemote
}

type LabelRemote struct {
	Label  string
	Remote string
}
