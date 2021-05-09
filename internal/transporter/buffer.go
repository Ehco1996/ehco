package transporter

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"syscall"

	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/web"
)

// 全局pool
var InboundBufferPool, OutboundBufferPool *sync.Pool

func init() {
	InboundBufferPool = NewBufferPool(constant.BUFFER_SIZE)
	OutboundBufferPool = NewBufferPool(constant.BUFFER_SIZE)
}

func NewBufferPool(size int) *sync.Pool {
	return &sync.Pool{New: func() interface{} {
		return make([]byte, size)
	}}
}

// NOTE adapeted from io.CopyBuffer
func copyBuffer(dst io.Writer, src io.Reader, bufferPool *sync.Pool) (written int64, err error) {
	buf := bufferPool.Get().([]byte)
	defer bufferPool.Put(buf)

	// If the reader has a WriteTo method, use it to do the copy.
	// Avoids an allocation and a copy.
	if wt, ok := src.(io.WriterTo); ok {
		written, err = wt.WriteTo(dst)
		web.NetWorkTransmitBytes.Add(float64(written))
		return
	}
	// Similarly, if the writer has a ReadFrom method, use it to do the copy.
	if rt, ok := dst.(io.ReaderFrom); ok {
		written, err = rt.ReadFrom(src)
		web.NetWorkTransmitBytes.Add(float64(written))
		return
	}
	for {
		nr, er := src.Read(buf)
		web.NetWorkTransmitBytes.Add(float64(nr * 2))
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}
	return
}

// NOTE must call setdeadline before use this func or may goroutine  leak
func transport(rw1, rw2 io.ReadWriter) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errc := make(chan error, 1)
	go func() {
		select {
		case <-ctx.Done():
			println("ctx done exits copy1")
		default:
			_, err := copyBuffer(rw1, rw2, InboundBufferPool)
			errc <- err
		}
	}()

	go func() {
		select {
		case <-ctx.Done():
			println("ctx done exit copy1")
		default:
			_, err := copyBuffer(rw2, rw1, InboundBufferPool)
			errc <- err
		}
	}()
	err := <-errc
	// NOTE 我们不关心operror 比如 eof/reset/broken pipe
	if err != nil {
		if err == io.EOF || errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) {
			err = nil
		}
		if _, ok := err.(*net.OpError); ok {
			err = nil
		}
	}
	return err
}

type BufferCh struct {
	Ch      chan []byte
	Handled bool
}

func newudpBufferCh() *BufferCh {
	return &BufferCh{
		Ch:      make(chan []byte, 100),
		Handled: false,
	}
}
