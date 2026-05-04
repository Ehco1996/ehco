package ms

import (
	"context"
	"errors"
	"os"
	"time"
)

// DBHealth is the storage + latency snapshot the Settings page polls.
// Sized small on purpose: every field is cheap (atomic load or one
// PRAGMA), so the handler can re-run on every refresh without a full
// COUNT(*) scan against the live tables.
type DBHealth struct {
	FileBytes       int64                      `json:"db_file_bytes"`
	PageCount       int64                      `json:"db_page_count"`
	PageSize        int64                      `json:"db_page_size"`
	FreelistPages   int64                      `json:"db_freelist_pages"`
	NodeMetricsRows int64                      `json:"node_metrics_rows"`
	RuleMetricsRows int64                      `json:"rule_metrics_rows"`
	LastRuleWriteTs int64                      `json:"last_rule_write_ts"`
	Stats           map[string]OpStatsSnapshot `json:"stats"`
}

func (ms *MetricsStore) Health(ctx context.Context) (*DBHealth, error) {
	h := &DBHealth{
		NodeMetricsRows: ms.nodeRows.Load(),
		RuleMetricsRows: ms.ruleRows.Load(),
		Stats:           ms.stats.Snapshot(),
	}
	if fi, err := os.Stat(ms.dbPath); err == nil {
		h.FileBytes = fi.Size()
	}
	if err := ms.db.QueryRowContext(ctx, "PRAGMA page_count").Scan(&h.PageCount); err != nil {
		return nil, err
	}
	if err := ms.db.QueryRowContext(ctx, "PRAGMA page_size").Scan(&h.PageSize); err != nil {
		return nil, err
	}
	if err := ms.db.QueryRowContext(ctx, "PRAGMA freelist_count").Scan(&h.FreelistPages); err != nil {
		return nil, err
	}
	// COALESCE keeps the JSON shape (always int64) even when the table
	// is empty — caller distinguishes "never written" via the 0 value.
	if err := ms.db.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(timestamp), 0) FROM rule_metrics").Scan(&h.LastRuleWriteTs); err != nil {
		return nil, err
	}
	return h, nil
}

// MaintenanceResult is the common shape returned by every maintenance
// op. Fields not relevant to a given op are left zero — Vacuum doesn't
// fill in NodeDeleted, Cleanup doesn't fill in BytesBefore, etc.
type MaintenanceResult struct {
	NodeDeleted int64 `json:"node_deleted,omitempty"`
	RuleDeleted int64 `json:"rule_deleted,omitempty"`
	BytesBefore int64 `json:"bytes_before,omitempty"`
	BytesAfter  int64 `json:"bytes_after,omitempty"`
	DurationMs  int64 `json:"duration_ms"`
}

// CleanupOlderThan deletes rows older than `days` from both metrics
// tables. days <= 0 falls back to the historical 30-day default.
func (ms *MetricsStore) CleanupOlderThan(ctx context.Context, days int) (*MaintenanceResult, error) {
	defer track(&ms.stats.Cleanup)()
	if days <= 0 {
		days = defaultRetentionDays
	}
	start := time.Now()
	cutoff := time.Now().AddDate(0, 0, -days).Unix()
	nodeDel, ruleDel, err := ms.deleteOlderThan(cutoff)
	if err != nil {
		return nil, err
	}
	_ = ctx // ctx kept for symmetry; deleteOlderThan uses ms.db directly
	return &MaintenanceResult{
		NodeDeleted: nodeDel,
		RuleDeleted: ruleDel,
		DurationMs:  time.Since(start).Milliseconds(),
	}, nil
}

// Vacuum reclaims free pages, blocking other queries for the duration.
// Cheap when the db is small (current ~2.5MB → <100ms); when it grows
// past ~1GB the lock window can stretch into multi-second territory —
// the SPA documents this in the confirm copy.
func (ms *MetricsStore) Vacuum(ctx context.Context) (*MaintenanceResult, error) {
	defer track(&ms.stats.Vacuum)()
	start := time.Now()
	before := ms.dbFileSize()
	if _, err := ms.db.ExecContext(ctx, "VACUUM"); err != nil {
		return nil, err
	}
	after := ms.dbFileSize()
	if err := ms.recountRows(); err != nil {
		return nil, err
	}
	ms.l.Infof("vacuum: %d -> %d bytes in %s", before, after, time.Since(start))
	return &MaintenanceResult{
		BytesBefore: before,
		BytesAfter:  after,
		DurationMs:  time.Since(start).Milliseconds(),
	}, nil
}

// ErrTruncateNotConfirmed is returned by Truncate when the caller does
// not pass the exact confirm literal. The handler turns this into a
// 400 so a missing form value can never wipe live data.
var ErrTruncateNotConfirmed = errors.New("truncate requires confirm=\"yes I am sure\"")

// truncateConfirm is the literal the API requires. Plain string, not
// boolean: a defaulted JSON field (`{}` → false) must not pass; only
// an explicit, typed phrase counts.
const truncateConfirm = "yes I am sure"

// Truncate empties both metrics tables and reclaims the freelist via
// VACUUM. The confirm string must match truncateConfirm exactly.
func (ms *MetricsStore) Truncate(ctx context.Context, confirm string) (*MaintenanceResult, error) {
	if confirm != truncateConfirm {
		return nil, ErrTruncateNotConfirmed
	}
	defer track(&ms.stats.Truncate)()
	start := time.Now()
	before := ms.dbFileSize()
	nodeBefore := ms.nodeRows.Load()
	ruleBefore := ms.ruleRows.Load()
	if _, err := ms.db.ExecContext(ctx, "DELETE FROM node_metrics"); err != nil {
		return nil, err
	}
	if _, err := ms.db.ExecContext(ctx, "DELETE FROM rule_metrics"); err != nil {
		return nil, err
	}
	if _, err := ms.db.ExecContext(ctx, "VACUUM"); err != nil {
		return nil, err
	}
	if err := ms.recountRows(); err != nil {
		return nil, err
	}
	after := ms.dbFileSize()
	ms.l.Warnf("truncate: deleted node=%d rule=%d, %d -> %d bytes",
		nodeBefore, ruleBefore, before, after)
	return &MaintenanceResult{
		NodeDeleted: nodeBefore,
		RuleDeleted: ruleBefore,
		BytesBefore: before,
		BytesAfter:  after,
		DurationMs:  time.Since(start).Milliseconds(),
	}, nil
}

// ResetStats zeroes every opStats counter. Operator escape hatch when a
// one-off latency spike (e.g. cold start, paused process) has poisoned
// the running max and the page is hard to read.
func (ms *MetricsStore) ResetStats() {
	ms.stats.Reset()
}

func (ms *MetricsStore) dbFileSize() int64 {
	if fi, err := os.Stat(ms.dbPath); err == nil {
		return fi.Size()
	}
	return 0
}
