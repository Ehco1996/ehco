package transporter

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

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
	gcTicker     *time.Ticker
}

func NewMWSSTransporter(l *zap.SugaredLogger) *mwssTransporter {
	tr := &mwssTransporter{
		sessionM: make(map[string][]*smux.Session),
		dialer: ws.Dialer{
			TLSConfig: mytls.DefaultTLSConfig,
			Timeout:   constant.DialTimeOut},
		L:        l,
		gcTicker: time.NewTicker(time.Second * 30),
	}
	// start gc thread for close idle sessions
	go tr.gc()
	return tr
}

func (tr *mwssTransporter) gc() {
	for range tr.gcTicker.C {
		tr.sessionMutex.Lock()
		for addr, sl := range tr.sessionM {
			tr.L.Debugf("doing gc for remote addr: %s total session count %d", addr, len(sl))
			for idx := range sl {
				tr.L.Debugf("check session: %s current stream count %d", sl[idx].LocalAddr().String(), sl[idx].NumStreams())
				if sl[idx].NumStreams() == 0 {
					sl[idx].Close()
					tr.L.Debugf("close idle session:%s", sl[idx].LocalAddr().String())
				}
			}
			newList := []*smux.Session{}
			for _, s := range sl {
				if !s.IsClosed() {
					newList = append(newList, s)
				}
			}
			tr.sessionM[addr] = newList
		}
		tr.sessionMutex.Unlock()
	}
}

func (tr *mwssTransporter) Dial(addr string) (conn net.Conn, err error) {
	tr.sessionMutex.Lock()
	defer tr.sessionMutex.Unlock()

	var session *smux.Session
	sessionList := tr.sessionM[addr]
	for _, s := range sessionList {
		if s.IsClosed() || s.NumStreams() >= constant.MaxMWSSStreamCnt {
			continue
		} else {
			tr.L.Debugf("use session: %s total stream count: %d", s.LocalAddr().String(), s.NumStreams())
			session = s
			break
		}
	}

	// create new one
	if session == nil {
		session, err = tr.initSession(addr)
		if err != nil {
			return nil, err
		}
		sessionList = append(sessionList, session)
		tr.sessionM[addr] = sessionList
	}

	stream, err := session.OpenStream()
	if err != nil {
		tr.L.Errorf("open stream meet error:%s", err)
		session.Close()
		return nil, err
	}
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
		s.L.Debugf("server err %s - %s : %s", conn.RemoteAddr(), s.Server.Addr, err)
		return
	}
	defer session.Close()

	s.L.Debugf("server init %s  %s", conn.RemoteAddr(), s.Server.Addr)
	defer s.L.Debugf("server close %s >-< %s", conn.RemoteAddr(), s.Server.Addr)

	for {
		stream, err := session.AcceptStream()
		if err != nil {
			s.L.Errorf("accept stream err: %s", err)
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
