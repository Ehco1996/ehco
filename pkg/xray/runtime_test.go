package xray

import (
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
}
