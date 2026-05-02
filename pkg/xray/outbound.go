package xray

import (
	"context"
	goerrors "errors"
	"io"
	"strconv"

	"github.com/xtls/xray-core/common"
	"github.com/xtls/xray-core/common/buf"
	"github.com/xtls/xray-core/common/errors"
	"github.com/xtls/xray-core/common/net"
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
		common.Interrupt(link.Reader)
		common.Interrupt(link.Writer)
		return
	}
	ob := obs[len(obs)-1]
	target := ob.Target
	if !target.IsValid() {
		common.Interrupt(link.Reader)
		common.Interrupt(link.Writer)
		return
	}
	ob.Name = "metered"

	// extract user identity from inbound session ctx
	var userID int
	var email string
	if inb := session.InboundFromContext(ctx); inb != nil && inb.User != nil {
		email = inb.User.Email
		if id, err := strconv.Atoi(email); err == nil {
			userID = id
		}
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
		common.Interrupt(link.Writer)
		common.Interrupt(link.Reader)
		return
	}

	network := "tcp"
	if target.Network == net.Network_UDP {
		network = "udp"
	}

	// pool may be nil if SyncTrafficEndPoint wasn't configured; counters disabled.
	var user *User
	if h.pool != nil && userID > 0 {
		user, _ = h.pool.GetUser(userID)
	}

	connID := h.tracker.Register(userID, email, network, target.NetAddr(), rawConn, cancel)
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
			common.Interrupt(link.Writer)
		}
	}
	common.Interrupt(link.Reader)
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
