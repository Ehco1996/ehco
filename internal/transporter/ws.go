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
	"github.com/Ehco1996/ehco/internal/constant"
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
)

type WsClient struct {
	dialer *ws.Dialer
	cfg    *conf.Config
	l      *zap.SugaredLogger
}

func newWsClient(cfg *conf.Config) (*WsClient, error) {
	s := &WsClient{
		cfg:    cfg,
		l:      zap.S().Named(string(cfg.TransportType)),
		dialer: &ws.Dialer{Timeout: cfg.Options.DialTimeout},
	}
	return s, nil
}

func (s *WsClient) getDialAddr(remote *lb.Node, isTCP bool) string {
	var addr string
	if !s.cfg.Options.NeedSendHandshakePayload() {
		addr = fmt.Sprintf("%s%s", remote.Address, HandshakePath)
	} else {
		addr = fmt.Sprintf("%s%s", s.cfg.Options.RemoteChains[0].Addr, DynamicHandShakePath)
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
	q.Set(QueryRelayType, constant.RelayUDP)
	u.RawQuery = q.Encode()
	return u.String()
}

func (s *WsClient) HandShake(ctx context.Context, remote *lb.Node, isTCP bool) (net.Conn, error) {
	startTime := time.Now()
	wsc, _, _, err := s.dialer.Dial(ctx, s.getDialAddr(remote, isTCP))
	if err != nil {
		return nil, fmt.Errorf("dial failed: %w", err)
	}

	if err := s.sendHandshakePayloadIfNeeded(wsc, remote.Address); err != nil {
		wsc.Close()
		return nil, err
	}

	latency := time.Since(startTime)
	s.recordMetrics(latency, isTCP, remote)
	return conn.NewWSConn(wsc, false), nil
}

func (s *WsClient) sendHandshakePayloadIfNeeded(wsc net.Conn, remoteAddr string) error {
	if !s.cfg.Options.NeedSendHandshakePayload() {
		return nil
	}

	payload := conf.BuildHandshakePayload(s.cfg.Options, remoteAddr)
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

func (s *WsClient) recordMetrics(latency time.Duration, isTCP bool, remote *lb.Node) {
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

type relayFunc func(context.Context, net.Conn, *lb.Node, constant.RelayType) error

func (s *WsServer) handleHandshake(e echo.Context) error {
	wsc, _, _, err := ws.UpgradeHTTP(e.Request(), e.Response())
	if err != nil {
		s.l.Errorf("Failed to upgrade HTTP connection: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to establish WebSocket connection")
	}
	defer wsc.Close()

	remote := s.remotes.Next()
	if remote == nil {
		return fmt.Errorf("no remote node available")
	}
	ctx := e.Request().Context()
	relayType := e.QueryParam(QueryRelayType)
	var f relayFunc
	if relayType == constant.RelayUDP {
		if !s.cfg.Options.EnableUDP {
			return fmt.Errorf("UDP not supported but requested")
		}
		f = s.RelayUDPConn
	} else {
		f = s.RelayTCPConn
	}
	if err := f(ctx, conn.NewWSConn(wsc, true), remote, s.cfg.TransportType); err != nil {
		s.l.Errorf("relay error: %v", err)
	}
	return nil
}

func (s *WsServer) handleDynamicHandshake(e echo.Context) error {
	wsc, _, _, err := ws.UpgradeHTTP(e.Request(), e.Response())
	if err != nil {
		s.l.Errorf("Failed to upgrade HTTP connection: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to establish WebSocket connection")
	}
	defer wsc.Close()

	// read payload
	payloadBytes, err := wsutil.ReadClientText(wsc)
	if err != nil {
		s.l.Errorf("Failed to read client message: %v", err)
		return nil
	}
	payload := &conf.HandshakePayload{}
	if err := json.Unmarshal(payloadBytes, payload); err != nil {
		s.l.Errorf("Failed to unmarshal payload: %v", err)
		return nil
	}
	s.l.Debugw("received handshake payload", "payload", payload)

	next, err := payload.RemoveLocalChainAndGetNext(s.cfg.Label)
	if err != nil {
		s.l.Errorf("Failed to remove local chain: %v", err)
		return nil
	}

	if next == nil {
		// no chain available, so relay conn to final addr
		remote := &lb.Node{Address: payload.FinalAddr}
		ctx := e.Request().Context()
		relayType := e.QueryParam(QueryRelayType)
		var f relayFunc
		if relayType == constant.RelayUDP {
			if !s.cfg.Options.EnableUDP {
				s.l.Errorf("UDP not supported but requested")
				return nil
			}
			f = s.RelayUDPConn
		} else {
			f = s.RelayTCPConn
		}
		return f(ctx, conn.NewWSConn(wsc, true), remote, s.cfg.TransportType)
	} else {
		// todo
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
