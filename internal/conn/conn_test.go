package conn

import (
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestInnerConn_ReadWrite(t *testing.T) {
	testData := []byte("hello")

	clientConn, serverConn := net.Pipe()
	clientConn.SetDeadline(time.Now().Add(1 * time.Second))
	serverConn.SetDeadline(time.Now().Add(1 * time.Second))
	defer clientConn.Close()
	defer serverConn.Close()

	innerC := &innerConn{Conn: clientConn, stats: &Stats{}, remoteLabel: "test"}

	errChan := make(chan error, 1)

	go func() {
		_, err := innerC.Write(testData)
		errChan <- err
	}()

	buf := make([]byte, len(testData))
	n, err := serverConn.Read(buf)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	assert.Equal(t, n, len(testData))
	assert.Equal(t, testData, buf)

	if err := <-errChan; err != nil {
		t.Fatalf("write err: %v", err)
	}
	assert.Equal(t, int64(len(testData)), innerC.stats.Up)

	errChan = make(chan error, 1)
	clientConn.SetDeadline(time.Now().Add(1 * time.Second))
	serverConn.SetDeadline(time.Now().Add(1 * time.Second))

	go func() {
		_, err := serverConn.Write(testData)
		errChan <- err // 将错误发送回主流程
	}()

	n, err = innerC.Read(buf)
	if err != nil {
		if errors.Is(err, io.EOF) {
			t.Logf("read eof")
		} else {
			t.Fatalf("read error: %v", err)
		}
	}
	assert.Equal(t, n, len(testData))
	assert.Equal(t, testData, buf)

	if err := <-errChan; err != nil {
		t.Fatalf("write error: %v", err)
	}
	assert.Equal(t, int64(len(testData)), innerC.stats.Down)
}

func TestCopyConn(t *testing.T) {
	// 设置监听端口，模拟外部服务器
	echoServer, err := net.Listen("tcp", "127.0.0.1:0") // 0 表示自动选择端口
	assert.NoError(t, err)
	defer echoServer.Close()

	// 数据准备
	msg := "Hello, TCP!"

	// 在另一个goroutine中启动回显服务器
	go func() {
		for {
			conn, err := echoServer.Accept()
			if err != nil {
				return // 避免无限制的错误日志
			}
			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c) // 简单的回显服务
			}(conn)
		}
	}()

	// 客户端连接到回显服务器
	clientConn, err := net.Dial("tcp", echoServer.Addr().String())
	assert.NoError(t, err)
	defer clientConn.Close()

	// 模拟内部连接，这里简化为再次使用客户端连接；实际使用中应为独立的连接
	remoteConn, err := net.Dial("tcp", echoServer.Addr().String())
	assert.NoError(t, err)
	defer remoteConn.Close()

	// 为 copyConn 函数创建包装后的连接
	c1 := &innerConn{Conn: clientConn, remoteLabel: "client", stats: &Stats{}}
	c2 := &innerConn{Conn: remoteConn, remoteLabel: "server", stats: &Stats{}}

	// 在goroutine中启动copyConn以避免阻塞
	go func() {
		if err := copyConn(c1, c2); err != nil {
			t.Log(err) // 使用t.Log而不是assert.NoError来避免在goroutine中调用t的方法
		}
	}()

	// 发送数据到客户端连接，观察是否能通过转发正确回显
	_, err = clientConn.Write([]byte(msg))
	assert.NoError(t, err)

	// 读取回显的数据
	buffer := make([]byte, len(msg))
	_, err = clientConn.Read(buffer)
	assert.NoError(t, err)
	assert.Equal(t, msg, string(buffer))
}
