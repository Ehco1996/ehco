package ms

const (
	MetricNodeCPU    = "node_cpu_usage"
	MetricNodeMem    = "node_memory_usage"
	MetricNodeDisk   = "node_disk_usage"
	MetricNodeNetIn  = "node_network_in"
	MetricNodeNetOut = "node_network_out"

	MetricRulePingMs      = "rule_ping_latency_ms"
	MetricRuleConnCount   = "rule_conn_count"
	MetricRuleHandshakeMs = "rule_handshake_ms"
	MetricRuleBytesTotal  = "rule_bytes_total"
)

const (
	LblLabel    = "label"
	LblRemote   = "remote"
	LblConnType = "conn_type"
	LblFlow     = "flow"
)

const (
	ConnTypeTCP = "tcp"
	ConnTypeUDP = "udp"
	FlowTx      = "tx"
	FlowRx      = "rx"
)

var nodeMetrics = []string{
	MetricNodeCPU, MetricNodeMem, MetricNodeDisk, MetricNodeNetIn, MetricNodeNetOut,
}
