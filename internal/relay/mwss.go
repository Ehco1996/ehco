package relay

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gobwas/ws"
	"github.com/xtaci/smux"
)

type mwssTransporter struct {
	sessions     map[string][]*muxSession
	sessionMutex sync.Mutex
	dialer       ws.Dialer
}

func NewMWSSTransporter() *mwssTransporter {
	return &mwssTransporter{
		sessions: make(map[string][]*muxSession),
		dialer:   ws.Dialer{TLSConfig: DefaultTLSConfig, Timeout: DialTimeOut},
	}
}

func (tr *mwssTransporter) Dial(addr string) (conn net.Conn, err error) {
	tr.sessionMutex.Lock()
	defer tr.sessionMutex.Unlock()

	var session *muxSession
	var sessionIndex int
	var sessions []*muxSession
	var ok bool

	sessions, ok = tr.sessions[addr]
	// 找到可以用的session
	for sessionIndex, session = range sessions {
		if session.NumStreams() >= session.maxStreamCnt {
			ok = false
		} else {
			ok = true
			break
		}
	}

	// 删除已经关闭的session
	if session != nil && session.IsClosed() {
		Logger.Infof("find closed session %v idx: %d", session, sessionIndex)
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
	cc, err := session.GetConn()
	if err != nil {
		session.Close()
		return nil, err
	}
	tr.sessions[addr] = sessions
	return cc, nil
}

func (tr *mwssTransporter) initSession(addr string) (*muxSession, error) {
	rc, _, _, err := tr.dialer.Dial(context.TODO(), addr)
	if err != nil {
		return nil, err
	}
	// stream multiplex
	smuxConfig := smux.DefaultConfig()
	session, err := smux.Client(rc, smuxConfig)
	if err != nil {
		return nil, err
	}
	Logger.Infof("[mwss] Init new session to: %s", rc.RemoteAddr())
	return &muxSession{conn: rc, session: session, maxStreamCnt: MaxMWSSStreamCnt}, nil
}

func (r *Relay) RunLocalMWSSServer() error {

	s := &MWSSServer{
		addr:     r.LocalTCPAddr.String(),
		connChan: make(chan net.Conn, 1024),
		errChan:  make(chan error, 1),
	}

	mux := http.NewServeMux()
	mux.Handle("/tcp/", http.HandlerFunc(s.upgrade))
	// fake
	mux.Handle("/", http.HandlerFunc(index))
	server := &http.Server{
		Addr:              r.LocalTCPAddr.String(),
		Handler:           mux,
		TLSConfig:         DefaultTLSConfig,
		ReadHeaderTimeout: 30 * time.Second,
	}
	s.server = server

	ln, err := net.Listen("tcp", r.LocalTCPAddr.String())
	if err != nil {
		return err
	}
	go func() {
		err := server.Serve(tls.NewListener(ln, server.TLSConfig))
		if err != nil {
			s.errChan <- err
		}
		close(s.errChan)
	}()

	var tempDelay time.Duration
	for {
		conn, e := s.Accept()
		if e != nil {
			if ne, ok := e.(net.Error); ok && ne.Temporary() {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				Logger.Infof("server: Accept error: %v; retrying in %v", e, tempDelay)
				time.Sleep(tempDelay)
				continue
			}
			return e
		}
		tempDelay = 0

		go r.handleMWSSConnToTcp(conn)
	}
}

type MWSSServer struct {
	addr     string
	server   *http.Server
	connChan chan net.Conn
	errChan  chan error
}

func (s *MWSSServer) upgrade(w http.ResponseWriter, r *http.Request) {
	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		Logger.Info(err)
		return
	}
	s.mux(conn)
}

func (s *MWSSServer) mux(conn net.Conn) {
	defer conn.Close()

	smuxConfig := smux.DefaultConfig()
	mux, err := smux.Server(conn, smuxConfig)
	if err != nil {
		Logger.Infof("[mwss server err] %s - %s : %s", conn.RemoteAddr(), s.Addr(), err)
		return
	}
	defer mux.Close()

	Logger.Infof("[mwss server init] %s  %s", conn.RemoteAddr(), s.Addr())
	defer Logger.Infof("[mwss server close] %s >-< %s", conn.RemoteAddr(), s.Addr())

	for {
		stream, err := mux.AcceptStream()
		if err != nil {
			Logger.Infof("[mwss] accept stream err: %s", err)
			break
		}
		cc := newMuxConn(conn, stream)
		select {
		case s.connChan <- cc:
		default:
			cc.Close()
			Logger.Infof("[mwss] %s - %s: connection queue is full", conn.RemoteAddr(), conn.LocalAddr())
		}
	}
}

func (s *MWSSServer) Accept() (conn net.Conn, err error) {
	select {
	case conn = <-s.connChan:
	case err = <-s.errChan:
	}
	return
}

func (s *MWSSServer) Close() error {
	return s.server.Close()
}

func (s *MWSSServer) Addr() string {
	return s.addr
}

func (r *Relay) handleTcpOverMWSS(c *net.TCPConn) error {
	defer c.Close()

	addr, node := r.PickTcpRemote()
	if node != nil {
		defer r.LBRemotes.DeferPick(node)
	}
	addr += "/tcp/"

	wsc, err := r.mwssTSP.Dial(addr)
	if err != nil {
		if r.EnableLB() && node != nil {
			// NOTE 向这个节点发请求挂了，负载的优先级降低
			r.LBRemotes.IncrUserCnt(node, 10000)
		}
		return err
	}
	defer wsc.Close()
	Logger.Infof("handleTcpOverMWSS from:%s to:%s", c.RemoteAddr(), wsc.RemoteAddr())
	transport(wsc, c)
	return nil
}

func (r *Relay) handleMWSSConnToTcp(c net.Conn) {
	defer c.Close()
	rc, err := net.Dial("tcp", r.RemoteTCPAddr)
	if err != nil {
		Logger.Infof("dial error: %s", err)
		return
	}
	defer rc.Close()
	Logger.Infof("handleMWSSConnToTcp from:%s to:%s", c.RemoteAddr(), rc.RemoteAddr())
	transport(rc, c)
}
