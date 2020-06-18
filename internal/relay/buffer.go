package relay

import (
	"io"
	"sync"
)

// 4KB
const BUFFER_SIZE = 4 * 1024

// 全局pool
var inboundBufferPool, outboundBufferPool *sync.Pool

func init() {
	inboundBufferPool = newBufferPool(BUFFER_SIZE)
	outboundBufferPool = newBufferPool(BUFFER_SIZE)
}

func newBufferPool(size int) *sync.Pool {
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

type udpBufferCh struct {
	Ch      chan []byte
	Handled bool
}

func newudpBufferCh() *udpBufferCh {
	return &udpBufferCh{
		Ch:      make(chan []byte, 100),
		Handled: false,
	}
}
