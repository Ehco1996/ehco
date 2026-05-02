package ms

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/nakabonne/tstorage"
	"go.uber.org/zap"
)

const writeTimeout = 0 // tstorage default

type MetricsStore struct {
	dir   string
	tsdb  tstorage.Storage
	idx   PairLister
	idxMu sync.RWMutex
	l     *zap.SugaredLogger

	wmCancel context.CancelFunc
}

// NewMetricsStore opens (or creates) tstorage at dataDir and starts the disk
// watermark goroutine bound to ctx. watermarkPct=0 selects the default (50%).
func NewMetricsStore(ctx context.Context, dataDir string, watermarkPct int) (*MetricsStore, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir tsdb dir: %w", err)
	}
	storage, err := tstorage.NewStorage(
		tstorage.WithDataPath(dataDir),
		tstorage.WithRetention(retention),
		tstorage.WithTimestampPrecision(tstorage.Seconds),
	)
	if err != nil {
		return nil, fmt.Errorf("open tstorage: %w", err)
	}

	wmCtx, cancel := context.WithCancel(ctx)
	ms := &MetricsStore{
		dir:      dataDir,
		tsdb:     storage,
		l:        zap.S().Named("ms"),
		wmCancel: cancel,
	}
	ms.startWatermark(wmCtx, watermarkPct)
	return ms, nil
}

func (ms *MetricsStore) SetPairLister(p PairLister) {
	ms.idxMu.Lock()
	ms.idx = p
	ms.idxMu.Unlock()
}

func (ms *MetricsStore) pairs(label, remote string) []LabelRemote {
	ms.idxMu.RLock()
	defer ms.idxMu.RUnlock()
	if ms.idx == nil {
		return nil
	}
	return ms.idx.Pairs(label, remote)
}

func (ms *MetricsStore) Close() error {
	if ms.wmCancel != nil {
		ms.wmCancel()
	}
	return ms.tsdb.Close()
}

func (ms *MetricsStore) DataDir() string { return ms.dir }

func (ms *MetricsStore) AddNodeMetric(ctx context.Context, s *NodeSnapshot) error {
	ts := s.SyncTime.Unix()
	return ms.tsdb.InsertRows([]tstorage.Row{
		{Metric: MetricNodeCPU, DataPoint: tstorage.DataPoint{Timestamp: ts, Value: s.CPUUsage}},
		{Metric: MetricNodeMem, DataPoint: tstorage.DataPoint{Timestamp: ts, Value: s.MemoryUsage}},
		{Metric: MetricNodeDisk, DataPoint: tstorage.DataPoint{Timestamp: ts, Value: s.DiskUsage}},
		{Metric: MetricNodeNetIn, DataPoint: tstorage.DataPoint{Timestamp: ts, Value: s.NetworkIn}},
		{Metric: MetricNodeNetOut, DataPoint: tstorage.DataPoint{Timestamp: ts, Value: s.NetworkOut}},
	})
}

func (ms *MetricsStore) AddRuleMetric(ctx context.Context, s *RuleSnapshot) error {
	ts := s.SyncTime.Unix()
	rows := make([]tstorage.Row, 0, len(s.Remotes)*9)
	for _, b := range s.Remotes {
		base := []tstorage.Label{
			{Name: LblLabel, Value: s.Label},
			{Name: LblRemote, Value: b.Remote},
		}
		rows = append(rows,
			row(MetricRulePingMs, base, ts, float64(b.PingLatencyMs)),

			row(MetricRuleConnCount, withConn(base, ConnTypeTCP), ts, float64(b.TCPConnCount)),
			row(MetricRuleConnCount, withConn(base, ConnTypeUDP), ts, float64(b.UDPConnCount)),

			row(MetricRuleHandshakeMs, withConn(base, ConnTypeTCP), ts, float64(b.TCPHandshakeMs)),
			row(MetricRuleHandshakeMs, withConn(base, ConnTypeUDP), ts, float64(b.UDPHandshakeMs)),

			row(MetricRuleBytesTotal, withConnFlow(base, ConnTypeTCP, FlowTx), ts, float64(b.TCPBytesTx)),
			row(MetricRuleBytesTotal, withConnFlow(base, ConnTypeTCP, FlowRx), ts, float64(b.TCPBytesRx)),
			row(MetricRuleBytesTotal, withConnFlow(base, ConnTypeUDP, FlowTx), ts, float64(b.UDPBytesTx)),
			row(MetricRuleBytesTotal, withConnFlow(base, ConnTypeUDP, FlowRx), ts, float64(b.UDPBytesRx)),
		)
	}
	return ms.tsdb.InsertRows(rows)
}

func (ms *MetricsStore) QueryNodeMetric(ctx context.Context, req *QueryNodeMetricsReq) (*QueryNodeMetricsResp, error) {
	type result struct {
		metric string
		points []*tstorage.DataPoint
		err    error
	}
	ch := make(chan result, len(nodeMetrics))
	for _, m := range nodeMetrics {
		go func(m string) {
			pts, err := ms.tsdb.Select(m, nil, req.StartTimestamp, req.EndTimestamp+1)
			ch <- result{m, pts, err}
		}(m)
	}

	byTS := make(map[int64]*NodeMetrics)
	for i := 0; i < len(nodeMetrics); i++ {
		r := <-ch
		if r.err != nil && !errors.Is(r.err, tstorage.ErrNoDataPoints) {
			return nil, fmt.Errorf("select %s: %w", r.metric, r.err)
		}
		for _, p := range r.points {
			nm, ok := byTS[p.Timestamp]
			if !ok {
				nm = &NodeMetrics{Timestamp: p.Timestamp}
				byTS[p.Timestamp] = nm
			}
			switch r.metric {
			case MetricNodeCPU:
				nm.CPUUsage = p.Value
			case MetricNodeMem:
				nm.MemoryUsage = p.Value
			case MetricNodeDisk:
				nm.DiskUsage = p.Value
			case MetricNodeNetIn:
				nm.NetworkIn = p.Value
			case MetricNodeNetOut:
				nm.NetworkOut = p.Value
			}
		}
	}

	out := make([]NodeMetrics, 0, len(byTS))
	for _, nm := range byTS {
		out = append(out, *nm)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp > out[j].Timestamp })
	if req.Num > 0 && int64(len(out)) > req.Num {
		out = out[:req.Num]
	}
	return &QueryNodeMetricsResp{TOTAL: len(out), Data: out}, nil
}

func (ms *MetricsStore) QueryRuleMetric(ctx context.Context, req *QueryRuleMetricsReq) (*QueryRuleMetricsResp, error) {
	pairs := ms.pairs(req.RuleLabel, req.Remote)
	if len(pairs) == 0 {
		return &QueryRuleMetricsResp{TOTAL: 0}, nil
	}

	rows := make([]RuleMetricsData, 0)
	for _, pr := range pairs {
		series, err := ms.fetchRuleSeries(pr, req.StartTimestamp, req.EndTimestamp+1)
		if err != nil {
			return nil, err
		}
		rows = append(rows, series...)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Timestamp > rows[j].Timestamp })
	if req.Num > 0 && int64(len(rows)) > req.Num {
		rows = rows[:req.Num]
	}
	return &QueryRuleMetricsResp{TOTAL: len(rows), Data: rows}, nil
}

func (ms *MetricsStore) fetchRuleSeries(pr LabelRemote, start, end int64) ([]RuleMetricsData, error) {
	base := []tstorage.Label{
		{Name: LblLabel, Value: pr.Label},
		{Name: LblRemote, Value: pr.Remote},
	}
	type query struct {
		metric string
		labels []tstorage.Label
		set    func(*RuleMetricsData, float64)
	}
	queries := []query{
		{MetricRulePingMs, base, func(r *RuleMetricsData, v float64) { r.PingLatency = int64(v) }},
		{MetricRuleConnCount, withConn(base, ConnTypeTCP), func(r *RuleMetricsData, v float64) { r.TCPConnectionCount = int64(v) }},
		{MetricRuleConnCount, withConn(base, ConnTypeUDP), func(r *RuleMetricsData, v float64) { r.UDPConnectionCount = int64(v) }},
		{MetricRuleHandshakeMs, withConn(base, ConnTypeTCP), func(r *RuleMetricsData, v float64) { r.TCPHandshakeDuration = int64(v) }},
		{MetricRuleHandshakeMs, withConn(base, ConnTypeUDP), func(r *RuleMetricsData, v float64) { r.UDPHandshakeDuration = int64(v) }},
		{MetricRuleBytesTotal, withConnFlow(base, ConnTypeTCP, FlowTx), func(r *RuleMetricsData, v float64) { r.TCPNetworkTransmitBytes = int64(v) }},
		{MetricRuleBytesTotal, withConnFlow(base, ConnTypeUDP, FlowTx), func(r *RuleMetricsData, v float64) { r.UDPNetworkTransmitBytes = int64(v) }},
	}

	type qr struct {
		idx    int
		points []*tstorage.DataPoint
		err    error
	}
	ch := make(chan qr, len(queries))
	for i, q := range queries {
		go func(i int, q query) {
			pts, err := ms.tsdb.Select(q.metric, q.labels, start, end)
			ch <- qr{i, pts, err}
		}(i, q)
	}

	byTS := make(map[int64]*RuleMetricsData)
	for i := 0; i < len(queries); i++ {
		r := <-ch
		if r.err != nil && !errors.Is(r.err, tstorage.ErrNoDataPoints) {
			return nil, fmt.Errorf("select rule %s: %w", queries[r.idx].metric, r.err)
		}
		for _, p := range r.points {
			row, ok := byTS[p.Timestamp]
			if !ok {
				row = &RuleMetricsData{Timestamp: p.Timestamp, Label: pr.Label, Remote: pr.Remote}
				byTS[p.Timestamp] = row
			}
			queries[r.idx].set(row, p.Value)
		}
	}

	out := make([]RuleMetricsData, 0, len(byTS))
	for _, row := range byTS {
		out = append(out, *row)
	}
	return out, nil
}

func row(metric string, labels []tstorage.Label, ts int64, v float64) tstorage.Row {
	return tstorage.Row{Metric: metric, Labels: labels, DataPoint: tstorage.DataPoint{Timestamp: ts, Value: v}}
}

func withConn(base []tstorage.Label, connType string) []tstorage.Label {
	out := make([]tstorage.Label, 0, len(base)+1)
	out = append(out, base...)
	out = append(out, tstorage.Label{Name: LblConnType, Value: connType})
	return out
}

func withConnFlow(base []tstorage.Label, connType, flow string) []tstorage.Label {
	out := make([]tstorage.Label, 0, len(base)+2)
	out = append(out, base...)
	out = append(out, tstorage.Label{Name: LblConnType, Value: connType})
	out = append(out, tstorage.Label{Name: LblFlow, Value: flow})
	return out
}

func LegacyDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".ehco", "metrics.db")
}
