package transporter

import (
	"context"
	"net"
	"net/http"
	"sync"

	"github.com/Ehco1996/ehco/internal/constant"
	mytls "github.com/Ehco1996/ehco/internal/tls"
	"github.com/gobwas/ws"
	"github.com/xtaci/smux"
	"go.uber.org/zap"
)

type mwssTransporter struct {
	sessionM     map[string][]*smux.Session
	sessionMutex sync.Mutex
	dialer       ws.Dialer
	L            *zap.SugaredLogger
}

func NewMWSSTransporter(l *zap.SugaredLogger) *mwssTransporter {
	return &mwssTransporter{
		sessionM: make(map[string][]*smux.Session),
		dialer: ws.Dialer{
			TLSConfig: mytls.DefaultTLSConfig,
			Timeout:   constant.DialTimeOut},
		L: l,
	}
}

func (tr *mwssTransporter) Dial(addr string) (conn net.Conn, err error) {
	tr.sessionMutex.Lock()
	defer tr.sessionMutex.Unlock()

	var session *smux.Session
	var sessionIndex int
	var sessions []*smux.Session
	var ok bool

	sessions, ok = tr.sessionM[addr]
	// 找到可以用的session
	for sessionIndex, session = range sessions {
		if session.NumStreams() >= constant.MaxMWSSStreamCnt {
			ok = false
		} else {
			ok = true
			break
		}
	}

	// 删除已经关闭的session
	if session != nil && session.IsClosed() {
		tr.L.Infof("find closed idx: %d", sessionIndex)
		sessions = append(sessions[:sessionIndex], sessions[sessionIndex+1:]...)
		ok = false
	}

	// 创建新的session
	if !ok {
		session, err = tr.initSession(addr)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	stream, err := session.OpenStream()
	if err != nil {
		session.Close()
		return nil, err
	}
	tr.sessionM[addr] = sessions
	return stream, nil
}

func (tr *mwssTransporter) initSession(addr string) (*smux.Session, error) {
	rc, _, _, err := tr.dialer.Dial(context.TODO(), addr)
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
	tr.L.Infof("Init new session to: %s", rc.RemoteAddr())
	return session, nil
}

type MWSSServer struct {
	Server   *http.Server
	ConnChan chan net.Conn
	ErrChan  chan error
	L        *zap.SugaredLogger
}

func NewMWSSServer(l *zap.SugaredLogger) *MWSSServer {
	return &MWSSServer{
		ConnChan: make(chan net.Conn, 1024),
		ErrChan:  make(chan error, 1),
		L:        l,
	}
}

func (s *MWSSServer) Upgrade(w http.ResponseWriter, r *http.Request) {
	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		s.L.Error(err)
		return
	}
	s.mux(conn)
}

func (s *MWSSServer) mux(conn net.Conn) {
	defer conn.Close()

	cfg := smux.DefaultConfig()
	cfg.KeepAliveDisabled = true
	session, err := smux.Server(conn, cfg)
	if err != nil {
		s.L.Infof("server err %s - %s : %s", conn.RemoteAddr(), s.Server.Addr, err)
		return
	}
	defer session.Close()

	s.L.Infof("server init %s  %s", conn.RemoteAddr(), s.Server.Addr)
	defer s.L.Infof("server close %s >-< %s", conn.RemoteAddr(), s.Server.Addr)

	for {
		stream, err := session.AcceptStream()
		if err != nil {
			s.L.Infof("accept stream err: %s", err)
			break
		}
		select {
		case s.ConnChan <- stream:
		default:
			stream.Close()
			s.L.Infof("%s - %s: connection queue is full", conn.RemoteAddr(), conn.LocalAddr())
		}
	}
}

func (s *MWSSServer) Accept() (conn net.Conn, err error) {
	select {
	case conn = <-s.ConnChan:
	case err = <-s.ErrChan:
	}
	return
}

func (s *MWSSServer) Close() error {
	return s.Server.Close()
}
