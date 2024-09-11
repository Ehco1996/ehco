package transporter

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/Ehco1996/ehco/internal/conn"
	"github.com/Ehco1996/ehco/internal/lb"
	"github.com/Ehco1996/ehco/internal/metrics"
	"github.com/Ehco1996/ehco/internal/relay/conf"
	"github.com/Ehco1996/ehco/internal/web"
)

var (
	_ RelayClient = &WsClient{}
	_ RelayServer = &WsServer{}
)

const (
	IndexPath            = "/"
	HandshakePath        = "/handshake"
	DynamicHandShakePath = "/dynamic_handshake"

	QueryRelayType = "relay_type"
	RelayTypeUDP   = "udp"
)

type contextKey string

const (
	contextKeyHandshakePayload contextKey = "handshake_payload"
)

type WsClient struct {
	dialer *ws.Dialer
	cfg    *conf.Config
	l      *zap.SugaredLogger
}

func newWsClient(cfg *conf.Config) (*WsClient, error) {
	return &WsClient{
		cfg:    cfg,
		l:      zap.S().Named(string(cfg.TransportType)),
		dialer: &ws.Dialer{Timeout: cfg.Options.DialTimeout},
	}, nil
}

func (s *WsClient) getDialAddr(remote *lb.Remote, isTCP bool) string {
	var addr string
	if !s.cfg.Options.NeedSendHandshakePayload() {
		addr = fmt.Sprintf("%s%s", remote.Address, HandshakePath)
	} else {
		addr = fmt.Sprintf("%s%s", s.cfg.Options.RemotesChain[0].Address, DynamicHandShakePath)
	}
	if !isTCP {
		addr = s.addUDPQueryParam(addr)
	}
	return addr
}

func (s *WsClient) addUDPQueryParam(addr string) string {
	u, err := url.Parse(addr)
	if err != nil {
		s.l.Errorf("Failed to parse URL: %v", err)
		return addr
	}
	q := u.Query()
	q.Set(QueryRelayType, RelayTypeUDP)
	u.RawQuery = q.Encode()
	return u.String()
}

func (s *WsClient) HandShake(ctx context.Context, remote *lb.Remote, isTCP bool) (net.Conn, error) {
	startTime := time.Now()
	wsc, _, _, err := s.dialer.Dial(ctx, s.getDialAddr(remote, isTCP))
	if err != nil {
		return nil, fmt.Errorf("dial failed: %w", err)
	}

	if err := s.sendHandshakePayloadIfNeeded(ctx, wsc, remote.Address); err != nil {
		wsc.Close()
		return nil, err
	}

	s.recordMetrics(time.Since(startTime), isTCP, remote)
	return conn.NewWSConn(wsc, false), nil
}

func (s *WsClient) sendHandshakePayloadIfNeeded(ctx context.Context, wsc net.Conn, remoteAddr string) error {
	var payload *conf.HandshakePayload

	if ctxPayload, ok := ctx.Value(contextKeyHandshakePayload).(*conf.HandshakePayload); ok && ctxPayload != nil {
		payload = ctxPayload
	} else if s.cfg.Options.NeedSendHandshakePayload() {
		payload = conf.BuildHandshakePayload(s.cfg.Options)
	} else {
		return nil
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload failed: %w", err)
	}

	if err := wsutil.WriteClientMessage(wsc, ws.OpText, payloadBytes); err != nil {
		return fmt.Errorf("write client message failed: %w", err)
	}

	s.l.Debugw("sent handshake payload", "remote", remoteAddr, "payload", payload)
	return nil
}

func (s *WsClient) recordMetrics(latency time.Duration, isTCP bool, remote *lb.Remote) {
	connType := metrics.METRIC_CONN_TYPE_TCP
	if !isTCP {
		connType = metrics.METRIC_CONN_TYPE_UDP
	}
	labels := []string{s.cfg.Label, connType, remote.Address}
	metrics.HandShakeDurationMilliseconds.WithLabelValues(labels...).Observe(float64(latency.Milliseconds()))
	remote.HandShakeDuration = latency
}

type WsServer struct {
	*BaseRelayServer
	httpServer *http.Server
}

func newWsServer(bs *BaseRelayServer) (*WsServer, error) {
	s := &WsServer{BaseRelayServer: bs}
	e := web.NewEchoServer()
	e.Use(web.NginxLogMiddleware(zap.S().Named("ws-server")))
	e.GET(IndexPath, echo.WrapHandler(web.MakeIndexF()))
	e.GET(HandshakePath, s.handleHandshake)
	e.GET(DynamicHandShakePath, s.handleDynamicHandshake)
	s.httpServer = &http.Server{Handler: e}
	return s, nil
}

func (s *WsServer) handleHandshake(e echo.Context) error {
	wsc, err := s.upgradeToWebSocket(e)
	if err != nil {
		return err
	}
	defer wsc.Close()

	remote := s.remotes.Next()
	if remote == nil {
		return fmt.Errorf("no remote node available")
	}
	return s.handleRelay(e.Request().Context(), wsc, remote, e.QueryParam(QueryRelayType) == RelayTypeUDP)
}

func (s *WsServer) handleDynamicHandshake(e echo.Context) error {
	wsc, err := s.upgradeToWebSocket(e)
	if err != nil {
		return err
	}
	defer wsc.Close()

	payload, err := s.readAndParseHandshakePayload(wsc)
	if err != nil {
		s.l.Errorf("Failed to read and parse handshake payload: %v", err)
		return err
	}

	remote := payload.PopNextRemote()
	if remote == nil {
		return fmt.Errorf("no remote node available")
	}

	ctx := e.Request().Context()
	if len(payload.RemotesChain) > 0 {
		ctx = context.WithValue(ctx, contextKeyHandshakePayload, payload.Clone())
	}

	return s.handleRelay(ctx, wsc, remote, e.QueryParam(QueryRelayType) == RelayTypeUDP)
}

func (s *WsServer) upgradeToWebSocket(e echo.Context) (net.Conn, error) {
	wsc, _, _, err := ws.UpgradeHTTP(e.Request(), e.Response())
	if err != nil {
		s.l.Errorf("Failed to upgrade HTTP connection: %v", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError, "Failed to establish WebSocket connection")
	}
	return wsc, nil
}

func (s *WsServer) handleRelay(ctx context.Context, wsc net.Conn, remote *lb.Remote, isUDP bool) error {
	relayF := s.RelayTCPConn
	if isUDP {
		if !s.cfg.Options.EnableUDP {
			return fmt.Errorf("UDP not supported but requested")
		}
		relayF = s.RelayUDPConn
	}
	if err := relayF(ctx, conn.NewWSConn(wsc, true), remote); err != nil {
		s.l.Errorf("relay error: %v", err)
	}
	return nil
}

func (s *WsServer) ListenAndServe(ctx context.Context) error {
	listener, err := NewTCPListener(ctx, s.cfg)
	if err != nil {
		return err
	}
	return s.httpServer.Serve(listener)
}

func (s *WsServer) Close() error {
	return s.httpServer.Close()
}

func (s *WsServer) readAndParseHandshakePayload(wsc net.Conn) (*conf.HandshakePayload, error) {
	msg, _, err := wsutil.ReadClientData(wsc)
	if err != nil {
		return nil, fmt.Errorf("read client data failed: %w", err)
	}

	var payload conf.HandshakePayload
	if err := json.Unmarshal(msg, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal payload failed: %w", err)
	}

	return &payload, nil
}
