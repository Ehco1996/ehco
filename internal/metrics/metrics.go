package metrics

// Public conn-type / flow constants. Re-exported with both old (METRIC_*) and
// new (ConnType*, Flow*) names so call sites can migrate at any pace.
const (
	METRIC_CONN_TYPE_TCP = ConnTypeTCP
	METRIC_CONN_TYPE_UDP = ConnTypeUDP
	METRIC_FLOW_READ     = FlowRx
	METRIC_FLOW_WRITE    = FlowTx
)
