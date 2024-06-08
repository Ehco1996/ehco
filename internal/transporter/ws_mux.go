// NOTE CAN NOT use real ws frame to transport smux frame
// err: accept stream err: buffer size:8 too small to transport ws payload size:45
// so this transport just use ws protocol to handshake and then use smux protocol to transport
package transporter

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/gobwas/ws"
	"github.com/labstack/echo/v4"
	"github.com/xtaci/smux"

	"github.com/Ehco1996/ehco/internal/metrics"
	"github.com/Ehco1996/ehco/pkg/lb"
)

var (
	_ RelayClient = &MwsClient{}
	_ RelayServer = &MwsServer{}
	_ muxServer   = &MwsServer{}
)

type MwsClient struct {
	*WssClient

	muxTP *smuxTransporter
}

func newMwsClient(base *baseTransporter) (*MwsClient, error) {
	wc, err := newWssClient(base)
	if err != nil {
		return nil, err
	}
	c := &MwsClient{WssClient: wc}
	c.muxTP = NewSmuxTransporter(c.l.Named("mwss"), c.initNewSession)
	return c, nil
}

func (c *MwsClient) initNewSession(ctx context.Context, addr string) (*smux.Session, error) {
	rc, _, _, err := c.dialer.Dial(ctx, addr)
	if err != nil {
		return nil, err
	}
	// stream multiplex
	cfg := smux.DefaultConfig()
	cfg.KeepAliveDisabled = true
	session, err := smux.Client(rc, cfg)
	if err != nil {
		return nil, err
	}
	c.l.Infof("init new session to: %s", rc.RemoteAddr())
	return session, nil
}

func (s *MwsClient) TCPHandShake(remote *lb.Node) (net.Conn, error) {
	t1 := time.Now()
	addr, err := s.cfg.GetWSRemoteAddr(remote.Address)
	if err != nil {
		return nil, err
	}
	mwssc, err := s.muxTP.Dial(context.TODO(), addr)
	if err != nil {
		return nil, err
	}
	latency := time.Since(t1)
	metrics.HandShakeDuration.WithLabelValues(remote.Label).Observe(float64(latency.Milliseconds()))
	remote.HandShakeDuration = latency
	return mwssc, nil
}

type MwsServer struct {
	*WsServer
	*muxServerImpl
}

func newMwsServer(base *baseTransporter) (*MwsServer, error) {
	wsServer, err := newWsServer(base)
	if err != nil {
		return nil, err
	}
	s := &MwsServer{
		WsServer:      wsServer,
		muxServerImpl: newMuxServer(base.cfg.Listen, base.l.Named("mwss")),
	}
	s.e.GET(base.cfg.GetWSHandShakePath(), echo.WrapHandler(http.HandlerFunc(s.HandleRequest)))
	return s, nil
}

func (s *MwsServer) ListenAndServe() error {
	go func() {
		s.errChan <- s.e.StartServer(s.httpServer)
	}()

	for {
		conn, e := s.Accept()
		if e != nil {
			return e
		}
		go func(c net.Conn) {
			if err := s.RelayTCPConn(c, s.relayer.TCPHandShake); err != nil {
				s.l.Errorf("RelayTCPConn error: %s", err.Error())
			}
		}(conn)
	}
}

func (s *MwsServer) HandleRequest(w http.ResponseWriter, r *http.Request) {
	c, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		s.l.Error(err)
		return
	}
	s.mux(c)
}

func (s *MwsServer) Close() error {
	return s.e.Close()
}
