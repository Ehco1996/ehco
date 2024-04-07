// nolint: errcheck
package transporter

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/xtaci/smux"
	"go.uber.org/zap"
)

type smuxTransporter struct {
	sessionMutex sync.Mutex

	gcTicker *time.Ticker
	l        *zap.SugaredLogger

	// remote addr -> SessionWithMetrics
	sessionM map[string][]*SessionWithMetrics

	initSessionF func(ctx context.Context, addr string) (*smux.Session, error)
}

type SessionWithMetrics struct {
	session *smux.Session

	createdTime time.Time
	streamList  []*smux.Stream
}

func (sm *SessionWithMetrics) CanNotServeNewStream() bool {
	return sm.session.IsClosed() ||
		sm.session.NumStreams() >= constant.SmuxMaxStreamCnt ||
		time.Since(sm.createdTime) > constant.SmuxMaxAliveDuration
}

func streamDead(s *smux.Stream) bool {
	select {
	case _, ok := <-s.GetDieCh():
		return !ok // 如果接收到值且通道未关闭，则 Stream 未死
	default:
		return true // 如果通道已经关闭，则 Stream 死了
	}
}

func (sm *SessionWithMetrics) canCloseSession(remoteAddr string, l *zap.SugaredLogger) bool {
	for _, s := range sm.streamList {
		if !streamDead(s) {
			return false
		}
		l.Debugf("session: %s stream: %d is not dead", remoteAddr, s.ID())
	}
	return true
}

func NewSmuxTransporter(
	l *zap.SugaredLogger,
	initSessionF func(ctx context.Context, addr string) (*smux.Session, error),
) *smuxTransporter {
	tr := &smuxTransporter{
		l:            l,
		initSessionF: initSessionF,
		sessionM:     make(map[string][]*SessionWithMetrics),
		gcTicker:     time.NewTicker(constant.SmuxGCDuration),
	}
	// start gc thread for close idle sessions
	go tr.gc()
	return tr
}

func (tr *smuxTransporter) gc() {
	for range tr.gcTicker.C {
		tr.sessionMutex.Lock()
		for addr, sl := range tr.sessionM {
			tr.l.Debugf("start doing gc for remote addr: %s total session count %d", addr, len(sl))
			for idx := range sl {
				sm := sl[idx]
				if sm.CanNotServeNewStream() && sm.canCloseSession(addr, tr.l) {
					tr.l.Debugf("close idle session:%s stream cnt %d",
						sm.session.LocalAddr().String(), sm.session.NumStreams())
					sm.session.Close()
				}
			}
			newList := []*SessionWithMetrics{}
			for _, s := range sl {
				if !s.session.IsClosed() {
					newList = append(newList, s)
				}
			}
			tr.sessionM[addr] = newList
			tr.l.Debugf("finish gc for remote addr: %s total session count %d", addr, len(sl))
		}
		tr.sessionMutex.Unlock()
	}
}

func (tr *smuxTransporter) Dial(ctx context.Context, addr string) (conn net.Conn, err error) {
	tr.sessionMutex.Lock()
	defer tr.sessionMutex.Unlock()
	var session *smux.Session
	var curSM *SessionWithMetrics

	sessionList := tr.sessionM[addr]
	for _, sm := range sessionList {
		if sm.CanNotServeNewStream() {
			continue
		} else {
			tr.l.Debugf("use session: %s total stream count: %d remote addr: %s",
				sm.session.LocalAddr().String(), sm.session.NumStreams(), addr)
			session = sm.session
			curSM = sm
			break
		}
	}
	// create new one
	if session == nil {
		session, err = tr.initSessionF(ctx, addr)
		if err != nil {
			return nil, err
		}
		sm := &SessionWithMetrics{session: session, createdTime: time.Now(), streamList: []*smux.Stream{}}
		sessionList = append(sessionList, sm)
		tr.sessionM[addr] = sessionList
		curSM = sm
	}

	stream, err := session.OpenStream()
	if err != nil {
		tr.l.Errorf("open stream meet error:%s", err)
		session.Close()
		return nil, err
	}
	curSM.streamList = append(curSM.streamList, stream)
	return stream, nil
}

type muxServer interface {
	ListenAndServe() error
	Accept() (net.Conn, error)
	Close() error
	mux(net.Conn)
}

func newMuxServer(listenAddr string, l *zap.SugaredLogger) *muxServerImpl {
	return &muxServerImpl{
		errChan:    make(chan error, 1),
		connChan:   make(chan net.Conn, 1024),
		listenAddr: listenAddr,
		l:          l,
	}
}

type muxServerImpl struct {
	errChan  chan error
	connChan chan net.Conn

	listenAddr string
	l          *zap.SugaredLogger
}

func (s *muxServerImpl) Accept() (net.Conn, error) {
	select {
	case conn := <-s.connChan:
		return conn, nil
	case err := <-s.errChan:
		return nil, err
	}
}

func (s *muxServerImpl) mux(conn net.Conn) {
	defer conn.Close()

	cfg := smux.DefaultConfig()
	cfg.KeepAliveDisabled = true
	session, err := smux.Server(conn, cfg)
	if err != nil {
		s.l.Debugf("server err %s - %s : %s", conn.RemoteAddr(), s.listenAddr, err)
		return
	}
	defer session.Close() // nolint: errcheck

	s.l.Debugf("session init %s  %s", conn.RemoteAddr(), s.listenAddr)
	defer s.l.Debugf("session close %s >-< %s", conn.RemoteAddr(), s.listenAddr)

	for {
		stream, err := session.AcceptStream()
		if err != nil {
			s.l.Errorf("accept stream err: %s", err)
			break
		}
		select {
		case s.connChan <- stream:
		default:
			stream.Close() // nolint: errcheck
			s.l.Infof("%s - %s: connection queue is full", conn.RemoteAddr(), conn.LocalAddr())
		}
	}
}
