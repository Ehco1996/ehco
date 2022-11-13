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
	L        *zap.SugaredLogger

	// remote addr -> SessionWithMetrics
	sessionM map[string][]*SessionWithMetrics

	initSessionF func(ctx context.Context, addr string) (*smux.Session, error)
}

type SessionWithMetrics struct {
	session     *smux.Session
	createdTime time.Time
}

func (sm *SessionWithMetrics) CanNotServe() bool {
	return sm.session.IsClosed() ||
		sm.session.NumStreams() >= constant.SmuxMaxStreamCnt ||
		time.Since(sm.createdTime) > constant.SmuxMaxAliveDuration
}

func NewSmuxTransporter(l *zap.SugaredLogger,
	initSessionF func(ctx context.Context, addr string) (*smux.Session, error)) *smuxTransporter {
	tr := &smuxTransporter{
		L:            l,
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
			tr.L.Debugf("==== start doing gc for remote addr: %s total session count %d ====", addr, len(sl))
			for idx := range sl {
				sm := sl[idx]
				tr.L.Debugf("check session: %s current stream count %d", sm.session.LocalAddr().String(), sm.session.NumStreams())
				if sm.session.NumStreams() == 0 {
					sm.session.Close()
					tr.L.Debugf("close idle session:%s", sm.session.LocalAddr().String())
				}
			}
			newList := []*SessionWithMetrics{}
			for _, s := range sl {
				if !s.session.IsClosed() {
					newList = append(newList, s)
				}
			}
			tr.sessionM[addr] = newList
			tr.L.Debugf("==== finish gc for remote addr: %s total session count %d ====", addr, len(sl))
		}
		tr.sessionMutex.Unlock()
	}
}

func (tr *smuxTransporter) Dial(ctx context.Context, addr string) (conn net.Conn, err error) {
	tr.sessionMutex.Lock()
	defer tr.sessionMutex.Unlock()
	var session *smux.Session

	sessionList := tr.sessionM[addr]
	for _, sm := range sessionList {
		if sm.CanNotServe() {
			continue
		} else {
			tr.L.Debugf("use session: %s total stream count: %d", sm.session.RemoteAddr().String(), sm.session.NumStreams())
			session = sm.session
			break
		}
	}

	// create new one
	if session == nil {
		session, err = tr.initSessionF(ctx, addr)
		if err != nil {
			return nil, err
		}
		sessionList = append(sessionList, &SessionWithMetrics{session: session, createdTime: time.Now()})
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
