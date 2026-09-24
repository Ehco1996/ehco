package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Ehco1996/ehco/internal/config"
	"github.com/Ehco1996/ehco/internal/sampler"
	"github.com/Ehco1996/ehco/internal/store"
)

func setupTestServer(t *testing.T, st *store.Store) *Server {
	t.Helper()
	cfg := &config.Config{
		WebPort: 18080,
	}
	s, err := NewServer(cfg, nil, nil, st)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	return s
}

func TestE2E_DataPipeline_FullLifecycle(t *testing.T) {
	dataDir := t.TempDir()
	st, err := store.NewStore(dataDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer func() { _ = st.Close() }()

	ctx := context.Background()

	// 1. Ingest synthetic samples across the last 15 minutes in chronological order
	now := time.Now()
	baseTime := now.Add(-15 * time.Minute)
	for i := 0; i < 30; i++ {
		ts := baseTime.Add(time.Duration(i*30) * time.Second)
		err := st.AddNodeMetric(ctx, &store.Sample{
			Timestamp:                ts.Unix(),
			CpuUsagePercent:          15.0 + float64(i%20),
			MemoryUsagePercent:       45.0,
			DiskUsagePercent:         30.0,
			NetworkReceiveBytesRate:  1024 * 100,
			NetworkTransmitBytesRate: 1024 * 200,
		})
		if err != nil {
			t.Fatalf("AddNodeMetric failed at %d: %v", i, err)
		}
	}

	// 2. Ingest latest sample via sampler.Collector.SampleOnce
	nodeSampler := sampler.NewNodeSampler()
	collector := sampler.NewCollector(st, nodeSampler)
	if err := collector.SampleOnce(ctx); err != nil {
		t.Fatalf("collector.SampleOnce failed: %v", err)
	}

	server := setupTestServer(t, st)

	// 3. Query /api/v1/node_metrics/ (raw samples)
	t.Run("GET /api/v1/node_metrics/ raw", func(t *testing.T) {
		start := baseTime.Unix()
		end := time.Now().Unix() + 10
		url := fmt.Sprintf("/api/v1/node_metrics/?start_ts=%d&end_ts=%d&step=0", start, end)
		req := httptest.NewRequest(http.MethodGet, url, nil)
		rec := httptest.NewRecorder()
		server.e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp store.QueryNodeMetricsResp
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}

		if resp.TOTAL <= 0 {
			t.Fatalf("expected TOTAL > 0, got %d", resp.TOTAL)
		}
		if len(resp.Data) == 0 {
			t.Fatalf("expected non-empty data points")
		}
		first := resp.Data[0]
		if first.CPUUsage <= 0 || first.MemoryUsage <= 0 || first.Timestamp <= 0 {
			t.Fatalf("unexpected point values: %+v", first)
		}
	})

	// 4. Query /api/v1/node_metrics/ with downsampling (step=60s)
	t.Run("GET /api/v1/node_metrics/ downsampled", func(t *testing.T) {
		start := baseTime.Unix()
		end := time.Now().Unix() + 10
		url := fmt.Sprintf("/api/v1/node_metrics/?start_ts=%d&end_ts=%d&step=60", start, end)
		req := httptest.NewRequest(http.MethodGet, url, nil)
		rec := httptest.NewRecorder()
		server.e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp store.QueryNodeMetricsResp
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp.TOTAL == 0 {
			t.Fatalf("expected downsampled buckets, got 0")
		}
	})

	// 5. Query /api/v1/overview
	t.Run("GET /api/v1/overview", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/overview", nil)
		rec := httptest.NewRecorder()
		server.e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var overview OverviewResp
		if err := json.Unmarshal(rec.Body.Bytes(), &overview); err != nil {
			t.Fatalf("failed to decode overview response: %v", err)
		}

		if overview.Host == nil {
			t.Fatalf("expected overview.Host to be populated, got nil")
		}
		if overview.Host.MemoryUsage <= 0 {
			t.Fatalf("expected non-zero MemoryUsage, got %f", overview.Host.MemoryUsage)
		}
	})

	// 6. Query /api/v1/db/health
	t.Run("GET /api/v1/db/health", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/db/health", nil)
		rec := httptest.NewRecorder()
		server.e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var health store.DBHealth
		if err := json.Unmarshal(rec.Body.Bytes(), &health); err != nil {
			t.Fatalf("failed to decode health response: %v", err)
		}

		if health.Partitions < 0 {
			t.Fatalf("expected partitions >= 0, got %d", health.Partitions)
		}
		if health.FileBytes < 0 {
			t.Fatalf("expected FileBytes >= 0, got %d", health.FileBytes)
		}
		if health.RetentionDays != 30 {
			t.Fatalf("expected RetentionDays=30, got %d", health.RetentionDays)
		}
		if health.PartitionDuration != "1h" {
			t.Fatalf("expected PartitionDuration=1h, got %s", health.PartitionDuration)
		}
		if health.NodeMetricsRows <= 0 {
			t.Fatalf("expected NodeMetricsRows > 0, got %d", health.NodeMetricsRows)
		}
	})

	// 7. Maintenance: POST /api/v1/db/reset_stats
	t.Run("POST /api/v1/db/reset_stats", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/db/reset_stats", nil)
		rec := httptest.NewRecorder()
		server.e.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204 NoContent, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	// 8. Maintenance: POST /api/v1/db/cleanup
	t.Run("POST /api/v1/db/cleanup", func(t *testing.T) {
		body := bytes.NewBufferString(`{"older_than_days": 30}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/db/cleanup", body)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var res store.MaintenanceResult
		if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to decode cleanup result: %v", err)
		}
	})

	// 9. Maintenance: POST /api/v1/db/truncate (rejection & success)
	t.Run("POST /api/v1/db/truncate rejection without valid confirm", func(t *testing.T) {
		body := bytes.NewBufferString(`{"confirm": "wrong_token"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/db/truncate", body)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.e.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d", rec.Code)
		}
	})

	t.Run("POST /api/v1/db/truncate success", func(t *testing.T) {
		body := bytes.NewBufferString(`{"confirm": "yes I am sure"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/db/truncate", body)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.e.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		// Verify data is cleared after truncate
		queryURL := fmt.Sprintf("/api/v1/node_metrics/?start_ts=%d&end_ts=%d",
			baseTime.Unix(), time.Now().Unix()+10)
		qReq := httptest.NewRequest(http.MethodGet, queryURL, nil)
		qRec := httptest.NewRecorder()
		server.e.ServeHTTP(qRec, qReq)

		var resp store.QueryNodeMetricsResp
		if err := json.Unmarshal(qRec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if resp.TOTAL != 0 {
			t.Fatalf("expected TOTAL=0 after truncate, got %d", resp.TOTAL)
		}
	})
}

func TestE2E_DataPipeline_StoreDisabled(t *testing.T) {
	// Server running with nil store (storage disabled)
	server := setupTestServer(t, nil)

	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/node_metrics/"},
		{http.MethodGet, "/api/v1/db/health"},
		{http.MethodPost, "/api/v1/db/cleanup"},
		{http.MethodPost, "/api/v1/db/truncate"},
		{http.MethodPost, "/api/v1/db/reset_stats"},
	}

	for _, ep := range endpoints {
		t.Run(fmt.Sprintf("%s %s returns 503", ep.method, ep.path), func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			rec := httptest.NewRecorder()
			server.e.ServeHTTP(rec, req)

			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("expected 503 Service Unavailable, got %d for %s %s", rec.Code, ep.method, ep.path)
			}
		})
	}
}
