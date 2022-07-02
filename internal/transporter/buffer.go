package transporter

import (
	"errors"
	"io"
	"net"
	"os"
	"syscall"
	"time"

	"go.uber.org/atomic"

	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/web"
	"github.com/Ehco1996/ehco/pkg/log"
	"github.com/xtaci/smux"
)

// 全局pool
var BufferPool *BytePool

func init() {
	BufferPool = NewBytePool(constant.BUFFER_POOL_SIZE, constant.BUFFER_SIZE)
}

// BytePool implements a leaky pool of []byte in the form of a bounded channel
type BytePool struct {
	c    chan []byte
	size int
}

// NewBytePool creates a new BytePool bounded to the given maxSize, with new
// byte arrays sized based on width.
func NewBytePool(maxSize int, size int) (bp *BytePool) {
	return &BytePool{
		c:    make(chan []byte, maxSize),
		size: size,
	}
}

// Get gets a []byte from the BytePool, or creates a new one if none are
// available in the pool.
func (bp *BytePool) Get() (b []byte) {
	select {
	case b = <-bp.c:
	// reuse existing buffer
	default:
		// create new buffer
		b = make([]byte, bp.size)
	}
	return
}

// Put returns the given Buffer to the BytePool.
func (bp *BytePool) Put(b []byte) {
	select {
	case bp.c <- b:
		// buffer went back into pool
	default:
		// buffer didn't go back into pool, just discard
	}
}

type ReadOnlyReader struct {
	io.Reader
}

type WriteOnlyWriter struct {
	io.Writer
}

// mute broken pipe connection reset timeout err.
func MuteErr(err error) error {
	if errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, smux.ErrTimeout) ||
		os.IsTimeout(err) || err == nil {
		return nil
	}
	return err
}

func transport(conn1, conn2 net.Conn, remote string) error {
	errCH := make(chan error, 1)
	// conn1 to conn2
	go func() {
		rn, err := io.Copy(WriteOnlyWriter{Writer: conn1}, ReadOnlyReader{Reader: conn2})
		web.NetWorkTransmitBytes.WithLabelValues(remote, web.METRIC_CONN_TCP).Add(float64(rn * 2))
		_ = conn1.SetReadDeadline(time.Now().Add(constant.IdleTimeOut)) // unblock read on conn1
		errCH <- err
	}()

	// conn2 to conn1
	rn, err := io.Copy(WriteOnlyWriter{Writer: conn2}, ReadOnlyReader{Reader: conn1})
	web.NetWorkTransmitBytes.WithLabelValues(remote, web.METRIC_CONN_TCP).Add(float64(rn * 2))
	if err2 := MuteErr(err); err2 != nil {
		log.Logger.Errorf("from:%s to:%s meet error:%s", conn2.LocalAddr(), conn1.RemoteAddr(), err2.Error())
	}
	_ = conn2.SetReadDeadline(time.Now().Add(constant.IdleTimeOut)) // unblock read on conn2
	return MuteErr(<-errCH)
}

type BufferCh struct {
	Ch      chan []byte
	Handled atomic.Bool
	UDPAddr *net.UDPAddr
}

func newudpBufferCh(clientUDPAddr *net.UDPAddr) *BufferCh {
	return &BufferCh{
		Ch:      make(chan []byte, 100),
		Handled: atomic.Bool{},
		UDPAddr: clientUDPAddr,
	}
}
