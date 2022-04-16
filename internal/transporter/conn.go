package transporter

import (
	"context"
	"net"
	"net/http"
	"sync"

	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/logger"
	mytls "github.com/Ehco1996/ehco/internal/tls"
	"github.com/gobwas/ws"
	"github.com/xtaci/smux"
)

type mwssTransporter struct {
	sessions     map[string][]*smux.Session
	sessionMutex sync.Mutex
	dialer       ws.Dialer
}

func NewMWSSTransporter() *mwssTransporter {
	return &mwssTransporter{
		sessions: make(map[string][]*smux.Session),
		dialer: ws.Dialer{
			TLSConfig: mytls.DefaultTLSConfig,
			Timeout:   constant.DialTimeOut},
	}
}

func (tr *mwssTransporter) Dial(addr string) (conn net.Conn, err error) {
	tr.sessionMutex.Lock()
	defer tr.sessionMutex.Unlock()

	var session *smux.Session
	var sessionIndex int
	var sessions []*smux.Session
	var ok bool

	sessions, ok = tr.sessions[addr]
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
		logger.Infof("find closed session %v idx: %d", session, sessionIndex)
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
	} else {
		if len(sessions) > 1 {
			// close last not used session, but we keep one conn in session pool
			if lastSession := sessions[len(sessions)-1]; lastSession.NumStreams() == 0 {
				lastSession.Close()
			}
		}
	}
	stream, err := session.OpenStream()
	if err != nil {
		session.Close()
		return nil, err
	}
	tr.sessions[addr] = sessions
	return stream, nil
}

func (tr *mwssTransporter) initSession(addr string) (*smux.Session, error) {
	rc, _, _, err := tr.dialer.Dial(context.TODO(), addr)
	if err != nil {
		return nil, err
	}
	// stream multiplex
	session, err := smux.Client(rc, nil)
	if err != nil {
		return nil, err
	}
	logger.Infof("[mwss] Init new session to: %s", rc.RemoteAddr())
	return session, nil
}

type MWSSServer struct {
	Server   *http.Server
	ConnChan chan net.Conn
	ErrChan  chan error
}

func NewMWSSServer() *MWSSServer {
	return &MWSSServer{
		ConnChan: make(chan net.Conn, 1024),
		ErrChan:  make(chan error, 1),
	}
}

func (s *MWSSServer) Upgrade(w http.ResponseWriter, r *http.Request) {
	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		logger.Info(err)
		return
	}
	s.mux(conn)
}

func (s *MWSSServer) mux(conn net.Conn) {
	defer conn.Close()

	session, err := smux.Server(conn, nil)
	if err != nil {
		logger.Infof("[mwss] server err %s - %s : %s", conn.RemoteAddr(), s.Server.Addr, err)
		return
	}
	defer session.Close()

	logger.Infof("[mwss] server init %s  %s", conn.RemoteAddr(), s.Server.Addr)
	defer logger.Infof("[mwss] server close %s >-< %s", conn.RemoteAddr(), s.Server.Addr)

	for {
		stream, err := session.AcceptStream()
		if err != nil {
			logger.Infof("[mwss] accept stream err: %s", err)
			break
		}
		select {
		case s.ConnChan <- stream:
		default:
			stream.Close()
			logger.Infof("[mwss] %s - %s: connection queue is full", conn.RemoteAddr(), conn.LocalAddr())
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
