package xray

import (
	xlog "github.com/xtls/xray-core/common/log"
	"go.uber.org/zap"
)

// zapBridgeHandler routes xray-core's log records into zap so they flow through
// the same WebSocket fan-out as ehco's own logs (see pkg/log).
//
// xray-core has a single global handler registered via xlog.RegisterHandler.
// We install this after core.New, replacing the default handler that writes
// to stdout/stderr. Severity is recovered by type-asserting Message to its
// concrete kind — the bare Message interface only exposes String().
type zapBridgeHandler struct {
	l *zap.Logger
}

func newZapBridgeHandler(l *zap.Logger) *zapBridgeHandler {
	return &zapBridgeHandler{l: l}
}

func (h *zapBridgeHandler) Handle(msg xlog.Message) {
	switch m := msg.(type) {
	case *xlog.GeneralMessage:
		h.logBySeverity(m.Severity, msg.String())
	case *xlog.AccessMessage:
		h.l.Info(msg.String(), zap.String("kind", "access"))
	case *xlog.DNSLog:
		h.l.Debug(msg.String(), zap.String("kind", "dns"))
	default:
		h.l.Info(msg.String())
	}
}

func (h *zapBridgeHandler) logBySeverity(s xlog.Severity, line string) {
	switch s {
	case xlog.Severity_Error:
		h.l.Error(line)
	case xlog.Severity_Warning:
		h.l.Warn(line)
	case xlog.Severity_Debug:
		h.l.Debug(line)
	default:
		// Unknown / Info — xray emits most lifecycle lines at Info.
		h.l.Info(line)
	}
}
