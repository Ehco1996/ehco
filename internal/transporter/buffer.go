package transporter

import (
	"io"
	"sync"

	"github.com/Ehco1996/ehco/internal/constant"
)

// 全局pool
var inboundBufferPool, outboundBufferPool *sync.Pool

func init() {
	inboundBufferPool = NewBufferPool(constant.BUFFER_SIZE)
	outboundBufferPool = NewBufferPool(constant.BUFFER_SIZE)
}

func NewBufferPool(size int) *sync.Pool {
	return &sync.Pool{New: func() interface{} {
		return make([]byte, size)
	}}
}

func copyBuffer(dst io.Writer, src io.Reader, bufferPool *sync.Pool) error {
	buf := bufferPool.Get().([]byte)
	defer bufferPool.Put(buf)
	_, err := io.CopyBuffer(dst, src, buf)
	return err
}

// NOTE must call setdeadline before use this func or may goroutine  leak
func transport(rw1, rw2 io.ReadWriter) error {
	errc := make(chan error, 1)
	go func() {
		errc <- copyBuffer(rw1, rw2, inboundBufferPool)
	}()

	go func() {
		errc <- copyBuffer(rw2, rw1, outboundBufferPool)
	}()

	err := <-errc
	if err != nil && err == io.EOF {
		err = nil
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
