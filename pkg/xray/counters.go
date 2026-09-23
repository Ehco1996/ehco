package xray

import "sync/atomic"

// counters is a flat, monotonic-since-start counter registry — the shape
// nginx's stub_status, HAProxy's `show stat` and Envoy's /stats all use:
// in-process, incremented where the event actually happens, served
// straight off the admin API with no external collector. Deliberately
// not persisted; a restart resets it, which is the semantics those tools
// have too.
//
// Errors are counted by the code path that produced them, never by
// parsing log text (Redis's errorstats works the same way).
type counters struct {
	connTotal    atomic.Int64
	connDialFail atomic.Int64
	configOK     atomic.Int64
	configFail   atomic.Int64
	syncOK       atomic.Int64
	syncFail     atomic.Int64
	reloadOK     atomic.Int64
	reloadFail   atomic.Int64
}

// snapshot renders the registry as the flat name -> value map served on
// /overview. Names are the counters' public API, so keep them stable.
func (c *counters) snapshot() map[string]int64 {
	return map[string]int64{
		"conn_total":        c.connTotal.Load(),
		"conn_dial_fail":    c.connDialFail.Load(),
		"config_fetch_ok":   c.configOK.Load(),
		"config_fetch_fail": c.configFail.Load(),
		"sync_ok":           c.syncOK.Load(),
		"sync_fail":         c.syncFail.Load(),
		"reload_ok":         c.reloadOK.Load(),
		"reload_fail":       c.reloadFail.Load(),
	}
}
