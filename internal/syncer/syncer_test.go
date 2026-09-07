package syncer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Ehco1996/ehco/internal/cmgr"
	"github.com/Ehco1996/ehco/internal/conn"
	"github.com/Ehco1996/ehco/internal/sampler"
)

type mockConn struct {
	label string
	stats conn.Stats
}

func (m *mockConn) Transport() error       { return nil }
func (m *mockConn) GetRelayLabel() string  { return m.label }
func (m *mockConn) GetStats() *conn.Stats { return &m.stats }
func (m *mockConn) Close() error           { return nil }

func TestSyncer_PushStats(t *testing.T) {
	var receivedReq syncReq
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&receivedReq); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mgr := cmgr.NewCmgr(true)
	c1 := &mockConn{label: "rule-1", stats: conn.Stats{Up: 500, Down: 1000, HandShakeLatency: 15 * time.Millisecond}}
	mgr.AddConnection(c1)
	mgr.RemoveConnection(c1)

	s := sampler.NewNodeSampler()
	sync := NewSyncer(server.URL, 60, mgr, s)

	ctx := context.Background()
	if err := sync.PushStats(ctx); err != nil {
		t.Fatalf("PushStats failed: %v", err)
	}

	if len(receivedReq.Stats) != 1 {
		t.Fatalf("expected 1 stat item, got %d", len(receivedReq.Stats))
	}
	stat := receivedReq.Stats[0]
	if stat.RelayLabel != "rule-1" || stat.Up != 500 || stat.Down != 1000 || stat.HandShakeLatency != 15 {
		t.Fatalf("unexpected stat content: %+v", stat)
	}
	if receivedReq.Node.CpuCoreCount <= 0 {
		t.Fatalf("expected node stats to be populated")
	}
}
