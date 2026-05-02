package xray

import (
	"testing"

	xlog "github.com/xtls/xray-core/common/log"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestZapBridgeHandler_GeneralMessageSeverity(t *testing.T) {
	cases := []struct {
		name    string
		sev     xlog.Severity
		wantLvl zapcore.Level
	}{
		{"error", xlog.Severity_Error, zapcore.ErrorLevel},
		{"warning", xlog.Severity_Warning, zapcore.WarnLevel},
		{"info", xlog.Severity_Info, zapcore.InfoLevel},
		{"debug", xlog.Severity_Debug, zapcore.DebugLevel},
		{"unknown_falls_back_to_info", xlog.Severity_Unknown, zapcore.InfoLevel},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			core, recorded := observer.New(zapcore.DebugLevel)
			h := newZapBridgeHandler(zap.New(core))

			h.Handle(&xlog.GeneralMessage{Severity: tc.sev, Content: "hello"})

			entries := recorded.All()
			if len(entries) != 1 {
				t.Fatalf("want 1 entry, got %d", len(entries))
			}
			if entries[0].Level != tc.wantLvl {
				t.Errorf("want level %s, got %s", tc.wantLvl, entries[0].Level)
			}
		})
	}
}

func TestZapBridgeHandler_AccessMessageIsInfoWithKind(t *testing.T) {
	core, recorded := observer.New(zapcore.DebugLevel)
	h := newZapBridgeHandler(zap.New(core))

	h.Handle(&xlog.AccessMessage{From: "1.2.3.4", To: "example.com", Status: xlog.AccessAccepted})

	entries := recorded.All()
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	if entries[0].Level != zapcore.InfoLevel {
		t.Errorf("want Info, got %s", entries[0].Level)
	}
	if got, ok := entries[0].ContextMap()["kind"]; !ok || got != "access" {
		t.Errorf(`want field kind="access", got %v`, entries[0].ContextMap())
	}
}
