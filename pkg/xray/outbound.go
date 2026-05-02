package xray

import (
	"context"
	goerrors "errors"
	"io"

	"github.com/xtls/xray-core/common"
	"github.com/xtls/xray-core/common/buf"
	"github.com/xtls/xray-core/common/errors"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/common/session"
	"github.com/xtls/xray-core/common/task"
	"github.com/xtls/xray-core/transport"
	"github.com/xtls/xray-core/transport/internet"
	"go.uber.org/zap"
)

// meteredOutbound implements outbound.Handler. It replaces xray's default
// freedom outbound so we can register every dialed conn into connTracker
// (for kill API) and accumulate per-user byte counts in the local UserPool
// (replacing xray's gRPC StatsService).
type meteredOutbound struct {
	tracker *connTracker
	pool    *UserPool
	l       *zap.Logger
}

func newMeteredOutbound(tracker *connTracker, pool *UserPool) *meteredOutbound {
	return &meteredOutbound{tracker: tracker, pool: pool, l: zap.L().Named("xray_outbound")}
}

func (h *meteredOutbound) Tag() string                          { return "" }
func (h *meteredOutbound) Start() error                         { return nil }
func (h *meteredOutbound) Close() error                         { return nil }
func (h *meteredOutbound) SenderSettings() *serial.TypedMessage { return nil }
func (h *meteredOutbound) ProxySettings() *serial.TypedMessage  { return nil }

// Dispatch implements outbound.Handler.
func (h *meteredOutbound) Dispatch(ctx context.Context, link *transport.Link) {
	obs := session.OutboundsFromContext(ctx)
	if len(obs) == 0 {
		_ = common.Interrupt(link.Reader)
		_ = common.Interrupt(link.Writer)
		return
	}
	ob := obs[len(obs)-1]
	target := ob.Target
	if !target.IsValid() {
		_ = common.Interrupt(link.Reader)
		_ = common.Interrupt(link.Writer)
		return
	}
	ob.Name = "metered"

	inb := session.InboundFromContext(ctx)
	userID := userIDFromInbound(inb)
	email := ""
	if inb != nil && inb.User != nil {
		email = inb.User.Email
	}

	dialCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	rawConn, err := internet.DialSystem(dialCtx, target, nil)
	if err != nil {
		h.l.Debug("dial failed",
			zap.String("target", target.NetAddr()),
			zap.String("email", email),
			zap.Error(err),
		)
		_ = common.Interrupt(link.Writer)
		_ = common.Interrupt(link.Reader)
		return
	}

	// pool may be nil if SyncTrafficEndPoint wasn't configured; counters disabled.
	var user *User
	if h.pool != nil && userID > 0 {
		user, _ = h.pool.GetUser(userID)
	}

	// Record the access IP for this cycle's traffic report. Source should
	// always be populated by xray on inbound accept; if it isn't, that's a
	// real anomaly worth surfacing.
	if user != nil {
		srcIP := sourceIPFromInbound(inb)
		if srcIP == "" {
			h.l.Warn("inbound source address missing, skipping IP record",
				zap.String("email", email),
			)
		} else {
			user.RecordIP(srcIP)
		}
	}

	connID := h.tracker.Register(inb, ob, rawConn, cancel)
	defer h.tracker.Unregister(connID)
	defer rawConn.Close()

	var reader buf.Reader = buf.NewReader(rawConn)
	var writer buf.Writer = buf.NewWriter(rawConn)
	if user != nil {
		reader = &meteringReader{inner: reader, user: user}
		writer = &meteringWriter{inner: writer, user: user}
	}

	requestDone := func() error {
		if err := buf.Copy(link.Reader, writer); err != nil {
			return errors.New("failed to send request").Base(err)
		}
		return nil
	}
	responseDone := func() error {
		if err := buf.Copy(reader, link.Writer); err != nil {
			return errors.New("failed to send response").Base(err)
		}
		return nil
	}

	if err := task.Run(dialCtx, requestDone, task.OnSuccess(responseDone, task.Close(link.Writer))); err != nil {
		errC := errors.Cause(err)
		if !goerrors.Is(errC, io.EOF) && !goerrors.Is(errC, io.ErrClosedPipe) && !goerrors.Is(errC, context.Canceled) {
			h.l.Debug("connection ended with error",
				zap.String("target", target.NetAddr()),
				zap.String("email", email),
				zap.Error(err),
			)
			_ = common.Interrupt(link.Writer)
		}
	}
	_ = common.Interrupt(link.Reader)
}

// meteringReader / meteringWriter wrap the dialed conn at the buf layer so
// every chunk going through bumps the user's atomic counter.
type meteringReader struct {
	inner buf.Reader
	user  *User
}

func (r *meteringReader) ReadMultiBuffer() (buf.MultiBuffer, error) {
	mb, err := r.inner.ReadMultiBuffer()
	r.user.AddDownloadTraffic(mbLen(mb) * 2) // ×2 = inbound + outbound pricing all vps provider to this
	return mb, err
}

type meteringWriter struct {
	inner buf.Writer
	user  *User
}

func (w *meteringWriter) WriteMultiBuffer(mb buf.MultiBuffer) error {
	w.user.AddUploadTraffic(mbLen(mb))
	return w.inner.WriteMultiBuffer(mb)
}

func mbLen(mb buf.MultiBuffer) int64 {
	var n int64
	for _, b := range mb {
		n += int64(b.Len())
	}
	return n
}
