package relay

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/xtaci/smux"
)

type muxStreamConn struct {
	net.Conn
	stream *smux.Stream
}

func (c *muxStreamConn) Read(b []byte) (n int, err error) {
	return c.stream.Read(b)
}

func (c *muxStreamConn) Write(b []byte) (n int, err error) {
	return c.stream.Write(b)
}

func (c *muxStreamConn) Close() error {
	return c.stream.Close()
}

type muxSession struct {
	conn         net.Conn
	session      *smux.Session
	maxStreamCnt int
}

func (session *muxSession) GetConn() (net.Conn, error) {
	stream, err := session.session.OpenStream()
	if err != nil {
		return nil, err
	}
	return &muxStreamConn{Conn: session.conn, stream: stream}, nil
}

func (session *muxSession) Accept() (net.Conn, error) {
	stream, err := session.session.AcceptStream()
	if err != nil {
		return nil, err
	}
	return &muxStreamConn{Conn: session.conn, stream: stream}, nil
}

func (session *muxSession) Close() error {
	if session.session == nil {
		return nil
	}
	return session.session.Close()
}

func (session *muxSession) IsClosed() bool {
	if session.session == nil {
		return true
	}
	return session.session.IsClosed()
}

func (session *muxSession) NumStreams() int {
	if session.session != nil {
		return session.session.NumStreams()
	}
	return 0
}

type mwssTransporter struct {
	sessions     map[string][]*muxSession
	sessionMutex sync.Mutex
}

func NewMWSSTransporter() *mwssTransporter {
	return &mwssTransporter{
		sessions: make(map[string][]*muxSession),
	}
}

func (tr *mwssTransporter) Dial(addr string) (conn net.Conn, err error) {
	tr.sessionMutex.Lock()
	defer tr.sessionMutex.Unlock()

	var session *muxSession
	var sessionIndex int
	sessions, ok := tr.sessions[addr]

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
	}

	// 创建新的session
	if !ok {
		u, err := url.Parse(addr)
		if err != nil {
			return nil, err
		}
		conn, err = net.DialTimeout("tcp", u.Host, WsDeadline)
		if err != nil {
			return nil, err
		}
		conn.SetDeadline(time.Now().Add(WsDeadline))

		session, err = tr.initSession(addr, conn)
		if err != nil {
			conn.Close()
			return nil, err
		}
		sessions = append(sessions, session)
	}

	cc, err := session.GetConn()
	if err != nil {
		session.Close()
		return nil, err
	}
	// TODO 统一管理session的deadline
	session.conn.SetDeadline(time.Now().Add(MWSSSessionDeadLine))
	session.session.SetDeadline(time.Now().Add(MWSSSessionDeadLine))
	tr.sessions[addr] = sessions
	return cc, nil
}

func (tr *mwssTransporter) initSession(addr string, conn net.Conn) (*muxSession, error) {
	d := websocket.Dialer{
		TLSClientConfig: DefaultTLSConfig,
		NetDial: func(net, addr string) (net.Conn, error) {
			return conn, nil
		}}
	u, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}
	c, resp, err := d.Dial(u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()
	wsc := newWsConn(c)
	// stream multiplex
	smuxConfig := smux.DefaultConfig()
	session, err := smux.Client(wsc, smuxConfig)
	if err != nil {
		return nil, err
	}
	Logger.Infof("[mwss] Init new session %s", session.RemoteAddr())
	return &muxSession{conn: wsc, session: session, maxStreamCnt: MaxMWSSStreamCnt}, nil
}

func (r *Relay) RunLocalMWSSServer() error {

	s := &MWSSServer{
		addr:     r.LocalTCPAddr.String(),
		upgrader: &websocket.Upgrader{},
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
	upgrader *websocket.Upgrader
	server   *http.Server
	connChan chan net.Conn
	errChan  chan error
}

func (s *MWSSServer) upgrade(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		Logger.Info(err)
		return
	}
	s.mux(newWsConn(conn))
}

func (s *MWSSServer) mux(conn net.Conn) {
	smuxConfig := smux.DefaultConfig()
	mux, err := smux.Server(conn, smuxConfig)
	if err != nil {
		Logger.Infof("[mwss] %s - %s : %s", conn.RemoteAddr(), s.Addr(), err)
		return
	}
	defer mux.Close()

	Logger.Infof("[mwss] %s <-> %s", conn.RemoteAddr(), s.Addr())
	defer Logger.Infof("[mwss] %s >-< %s", conn.RemoteAddr(), s.Addr())

	failedCount := 0
	for failedCount < 5 {
		stream, err := mux.AcceptStream()
		if err != nil {
			Logger.Infof("[mwss] accept stream err: %s failedCount: %d", err, failedCount)
			failedCount++
			break
		}
		cc := &muxStreamConn{Conn: conn, stream: stream}
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

var tr = NewMWSSTransporter()

func (r *Relay) handleTcpOverMWSS(c *net.TCPConn) error {
	defer c.Close()

	addr := r.RemoteTCPAddr + "/tcp/"
	wsc, err := tr.Dial(addr)
	if err != nil {
		return err
	}
	defer wsc.Close()
	Logger.Infof("handleTcpOverMWSS from:%s to:%s", c.RemoteAddr(), wsc.RemoteAddr())
	if err := wsc.SetDeadline(time.Now().Add(TransportDeadLine)); err != nil {
		return err
	}
	if err := c.SetDeadline(time.Now().Add(TransportDeadLine)); err != nil {
		return err
	}
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
	if err := rc.SetDeadline(time.Now().Add(TransportDeadLine)); err != nil {
		Logger.Infof("set deadline error: %s", err)
		return
	}
	if err := c.SetDeadline(time.Now().Add(TransportDeadLine)); err != nil {
		Logger.Infof("set deadline error: %s", err)
		return
	}
	transport(rc, c)
}
