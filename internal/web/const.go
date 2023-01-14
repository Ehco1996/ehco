package web

const (
	METRIC_NS                = "ehco"
	METRIC_SUBSYSTEM_TRAFFIC = "traffic"
	METRIC_SUBSYSTEM_PING    = "ping"

	METRIC_LABEL_REMOTE = "remote"

	METRIC_LABEL_CONN_FLOW = "flow"
	METRIC_CONN_FLOW_WRITE = "write"
	METRIC_CONN_FLOW_READ  = "read"

	METRIC_LABEL_CONN_TYPE = "type"
	METRIC_CONN_TYPE_TCP   = "tcp"
	METRIC_CONN_TYPE_UDP   = "udp"

	EhcoAliveStateInit    = 0
	EhcoAliveStateRunning = 1
)
