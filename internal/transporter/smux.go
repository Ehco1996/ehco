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
	sessionM map[string][]*smux.Session

	initSessionF func(ctx context.Context, addr string) (*smux.Session, error)
}

func NewSmuxTransporter(l *zap.SugaredLogger,
	initSessionF func(ctx context.Context, addr string) (*smux.Session, error)) *smuxTransporter {
	tr := &smuxTransporter{
		L:            l,
		initSessionF: initSessionF,
		sessionM:     make(map[string][]*smux.Session),
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
	for _, s := range sessionList {
		if s.IsClosed() || s.NumStreams() >= constant.MaxMuxStreamCnt {
			continue
		} else {
			tr.L.Debugf("use session: %s total stream count: %d", s.RemoteAddr().String(), s.NumStreams())
			session = s
			break
		}
	}

	// create new one
	if session == nil {
		session, err = tr.initSessionF(ctx, addr)
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
