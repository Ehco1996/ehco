package relay

import (
	"io"
	"log"
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

func doCopy(dst io.Writer, src io.Reader, bufferPool *sync.Pool, wg *sync.WaitGroup) {
	buf := bufferPool.Get().([]byte)
	defer bufferPool.Put(buf)
	_, err := io.CopyBuffer(dst, src, buf)
	if err != nil && err != io.EOF {
		log.Printf("failed to relay: %v\n", err)
	}
	wg.Done()
}
