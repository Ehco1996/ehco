package transporter

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"

	"go.uber.org/atomic"
	"go.uber.org/zap"

	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/web"
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

type ReadOnlyMetricsReader struct {
	io.Reader
	remoteLabel string
}

func (r ReadOnlyMetricsReader) Read(p []byte) (n int, err error) {
	n, err = r.Reader.Read(p)
	web.NetWorkTransmitBytes.WithLabelValues(
		r.remoteLabel, web.METRIC_CONN_TYPE_TCP, web.METRIC_CONN_FLOW_READ,
	).Add(float64(n))
	return
}

type WriteOnlyMetricsWriter struct {
	io.Writer
	remoteLabel string
}

func (w WriteOnlyMetricsWriter) Write(p []byte) (n int, err error) {
	n, err = w.Writer.Write(p)
	web.NetWorkTransmitBytes.WithLabelValues(
		w.remoteLabel, web.METRIC_CONN_TYPE_TCP, web.METRIC_CONN_FLOW_WRITE,
	).Add(float64(n))
	return
}

func shortHashSHA256(input string) string {
	hasher := sha256.New()
	hasher.Write([]byte(input))
	hash := hasher.Sum(nil)
	return hex.EncodeToString(hash)[:7]
}
func connectionName(conn net.Conn) string {
	return fmt.Sprintf("l:<%s> r:<%s>", conn.LocalAddr(), conn.RemoteAddr())
}

// 注意此代码假设 conn1 是和客户端的连接，conn2 是和远程服务器的连接。
// 在每个 io.Copy 完成后，我们通过判断连接是否是 *net.TCPConn 类型，
// 使用 CloseWrite 来关闭写方向的连接以发送 EOF。
// 接下来我们从错误通道 errCH 中接收另一个方向的错误，关闭剩余的读方向连接，并返回适当的错误信息。
func transport(conn1, conn2 net.Conn, remote string) error {
	name := fmt.Sprintf("c1:[%s] c2:[%s]", connectionName(conn1), connectionName(conn2))
	l := zap.S().Named(shortHashSHA256(name))
	l.Debugf("transport for:%s start", name)
	defer l.Debugf("transport for:%s end", name)

	errCH := make(chan error, 1)
	// conn1 to conn2
	go func() {
		l.Debug("copy conn1 to conn2 start")
		_, err := io.Copy(
			WriteOnlyMetricsWriter{Writer: conn2, remoteLabel: remote},
			ReadOnlyMetricsReader{Reader: conn1, remoteLabel: remote},
		)
		l.Debug("copy conn1 to conn2 end", err)
		if tcpConn, ok := conn2.(*net.TCPConn); ok {
			tcpConn.CloseWrite() // 立即发送EOF
		}
		errCH <- err
	}()

	// conn2 to conn1
	l.Debug("copy conn2 to conn1 start")
	_, err := io.Copy(
		WriteOnlyMetricsWriter{Writer: conn1, remoteLabel: remote},
		ReadOnlyMetricsReader{Reader: conn2, remoteLabel: remote},
	)
	l.Debug("copy conn2 to conn1 end", err)
	if tcpConn, ok := conn1.(*net.TCPConn); ok {
		tcpConn.CloseWrite() // 立即发送EOF
	}

	// 等待 conn1 到 conn2 goroutine 的错误
	err2 := <-errCH

	// 如果两个连接都是TCP连接，关闭读端
	if tcpConn, ok := conn1.(*net.TCPConn); ok {
		tcpConn.CloseRead()
	}
	if tcpConn, ok := conn2.(*net.TCPConn); ok {
		tcpConn.CloseRead()
	}

	// 此外，在处理错误时，我们也需要同时考虑两个方向的错误。
	// 如果两个方向都有错误，我们可以把两个错误都返回。
	// 如果只有一个方向有错误，我们只返回那个错误。如果两个方向都没有错误
	if err != nil && err2 != nil {
		return fmt.Errorf("errors in both directions: %v, %v", err, err2)
	}
	if err != nil {
		return err
	}
	return err2
}
