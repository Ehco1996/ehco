package xray

import (
	"errors"
	"net"
	"testing"
)

func TestRunningInbounds(t *testing.T) {
	xs := NewXrayServer(makeTestXrayConfig(t, XrayTrojanProxyTag, 10011))
	if err := xs.Setup(); err != nil {
		t.Fatalf("setup: %v", err)
	}
	got := xs.RunningInbounds()
	if got[XrayTrojanProxyTag] != "127.0.0.1,10011" {
		t.Fatalf("unexpected running inbounds: %+v", got)
	}
	if xs.Drifted() {
		t.Fatalf("fresh setup should not be drifted")
	}
}

func TestDriftOnPortChange(t *testing.T) {
	xs := NewXrayServer(makeTestXrayConfig(t, XrayTrojanProxyTag, 10021))
	if err := xs.Setup(); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// A newer config asks for a different port -> needReload flags drift.
	need, err := xs.needReload(makeTestXrayConfig(t, XrayTrojanProxyTag, 10022))
	if err != nil {
		t.Fatalf("needReload: %v", err)
	}
	if !need {
		t.Fatalf("expected needReload=true for changed port")
	}
	if !xs.Drifted() {
		t.Fatalf("expected Drifted()=true after a detected port change")
	}
}

func TestReloadClearsDrift(t *testing.T) {
	// Reserve a free port so the reload's bind is deterministic.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	xs := NewXrayServer(makeTestXrayConfig(t, XrayTrojanProxyTag, port))
	if err := xs.Setup(); err != nil {
		t.Fatalf("setup: %v", err)
	}
	xs.drift.Store(true)

	if err := xs.Reload(false); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if xs.Drifted() {
		t.Fatalf("a successful reload should clear drift")
	}
	events := xs.RecentEvents()
	if len(events) != 1 || events[0].Kind != "reload_ok" {
		t.Fatalf("expected a single reload_ok event, got %+v", events)
	}
	if c := xs.Counters(); c["reload_ok"] != 1 || c["reload_fail"] != 0 {
		t.Fatalf("unexpected reload counters: %v", c)
	}
}

func TestReloadFailureCounts(t *testing.T) {
	// Occupy a port so the reload's bind fails deterministically.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	xs := NewXrayServer(makeTestXrayConfig(t, XrayTrojanProxyTag, port))
	if err := xs.Setup(); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := xs.Reload(false); err == nil {
		t.Fatalf("expected the reload to fail on an occupied port")
	}
	c := xs.Counters()
	if c["reload_fail"] != 1 || c["reload_ok"] != 0 {
		t.Fatalf("unexpected reload counters: %v", c)
	}
}

func TestCountersStartAtZero(t *testing.T) {
	xs := NewXrayServer(makeTestXrayConfig(t, XrayTrojanProxyTag, 10061))
	c := xs.Counters()
	want := []string{
		"conn_total", "conn_dial_fail",
		"config_fetch_ok", "config_fetch_fail",
		"sync_ok", "sync_fail", "reload_ok", "reload_fail",
	}
	if len(c) != len(want) {
		t.Fatalf("expected %d counters, got %v", len(want), c)
	}
	for _, name := range want {
		if v, ok := c[name]; !ok || v != 0 {
			t.Fatalf("counter %q: want 0, got %d (present=%v)", name, v, ok)
		}
	}
}

func TestDriftEventLoggedOncePerEpisode(t *testing.T) {
	xs := NewXrayServer(makeTestXrayConfig(t, XrayTrojanProxyTag, 10041))
	if err := xs.Setup(); err != nil {
		t.Fatalf("setup: %v", err)
	}
	changed := makeTestXrayConfig(t, XrayTrojanProxyTag, 10042)

	if _, err := xs.needReload(changed); err != nil {
		t.Fatalf("needReload: %v", err)
	}
	// Same episode: drift is already set, so no second event.
	if _, err := xs.needReload(changed); err != nil {
		t.Fatalf("needReload: %v", err)
	}

	events := xs.RecentEvents()
	if len(events) != 1 || events[0].Kind != "drift" {
		t.Fatalf("expected exactly one drift event, got %+v", events)
	}
	if events[0].Detail == "" {
		t.Fatalf("drift event should describe the change")
	}
}

func TestSyncStatus(t *testing.T) {
	xs := NewXrayServer(makeTestXrayConfig(t, XrayTrojanProxyTag, 10051))
	if got := xs.ConfigSync(); !got.At.IsZero() {
		t.Fatalf("expected never-synced zero status, got %+v", got)
	}
	xs.configSync.record(errors.New("boom"))
	if got := xs.ConfigSync(); got.OK || got.Error != "boom" || got.At.IsZero() {
		t.Fatalf("unexpected error status: %+v", got)
	}
	xs.configSync.record(nil)
	if got := xs.ConfigSync(); !got.OK || got.Error != "" || got.At.IsZero() {
		t.Fatalf("unexpected ok status: %+v", got)
	}
	// Traffic sync with no user pool reports the zero value, not a stale one.
	if got := xs.TrafficSync(); !got.At.IsZero() {
		t.Fatalf("expected zero traffic status without a user pool, got %+v", got)
	}
}
