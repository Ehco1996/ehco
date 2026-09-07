package xray

import (
	"fmt"
	"testing"

	"github.com/Ehco1996/ehco/internal/config"
	xConf "github.com/xtls/xray-core/infra/conf"
)

func makeTestXrayConfig(t *testing.T, tag string, port int) *config.Config {
	t.Helper()
	inboundJSON := fmt.Sprintf(`{
		"listen": "127.0.0.1", "port": %d, "protocol": "trojan", "tag": %q,
		"settings": {"clients": [{"password": "pwd", "email": "1"}]},
		"streamSettings": {"network": "tcp", "security": "tls", "tlsSettings": {}}
	}`, port, tag)
	return parseConfig(t, "", inboundJSON)
}

func TestNeedReload_PortChange(t *testing.T) {
	cfg1 := makeTestXrayConfig(t, XrayTrojanProxyTag, 10001)
	xs := NewXrayServer(cfg1)
	if err := xs.Setup(); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// 1. Same port and tag -> no reload needed
	cfgSame := makeTestXrayConfig(t, XrayTrojanProxyTag, 10001)
	need, err := xs.needReload(cfgSame)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if need {
		t.Fatalf("expected needReload=false for same port, got true")
	}

	// 2. Different port -> reload needed
	cfgNewPort := makeTestXrayConfig(t, XrayTrojanProxyTag, 10002)
	need, err = xs.needReload(cfgNewPort)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !need {
		t.Fatalf("expected needReload=true for changed port, got false")
	}

	// 3. Pointer overwrite immunity test:
	// Even if xs.cfg is mutated concurrently (e.g. by relay server reloader),
	// needReload compares against runningInbounds, so it still correctly detects changes.
	xs.cfg.XRayConfig = cfgNewPort.XRayConfig
	// Checking with cfgNewPort should STILL return true because the running instance is still on 10001!
	need, err = xs.needReload(cfgNewPort)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !need {
		t.Fatalf("expected needReload=true even after xs.cfg pointer mutation, got false")
	}
}

func TestNeedReload_TagChange(t *testing.T) {
	cfg1 := makeTestXrayConfig(t, XrayTrojanProxyTag, 10001)
	xs := NewXrayServer(cfg1)
	if err := xs.Setup(); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Different tag
	cfgDiffTag := makeTestXrayConfig(t, XrayVlessProxyTag, 10001)
	need, err := xs.needReload(cfgDiffTag)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !need {
		t.Fatalf("expected needReload=true for changed tag, got false")
	}

	// Empty config
	need, err = xs.needReload(nil)
	if err != nil || need {
		t.Fatalf("expected false, nil for nil config, got %v, %v", need, err)
	}
	need, err = xs.needReload(&config.Config{XRayConfig: &xConf.Config{}})
	if err != nil || !need {
		t.Fatalf("expected needReload=true when inbounds removed, got %v, %v", need, err)
	}
}
