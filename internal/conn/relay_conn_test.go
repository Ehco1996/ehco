package conn

import (
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/Ehco1996/ehco/internal/lb"
	"github.com/Ehco1996/ehco/internal/relay/conf"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestInnerConn_ReadWrite(t *testing.T) {
	testData := []byte("hello")
	testOptions := conf.Options{
		IdleTimeout: time.Second,
		ReadTimeout: time.Second,
	}

	clientConn, serverConn := net.Pipe()
	clientConn.SetDeadline(time.Now().Add(1 * time.Second))
	serverConn.SetDeadline(time.Now().Add(1 * time.Second))
	defer clientConn.Close()
	defer serverConn.Close()
	rc := relayConnImpl{Stats: &Stats{}, remote: &lb.Node{}, Options: &testOptions}
	innerC := newInnerConn(clientConn, &rc)
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
	assert.Equal(t, int64(len(testData)), rc.Stats.Up)

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
	assert.Equal(t, int64(len(testData)), rc.Stats.Down)
}

func TestCopyTCPConn(t *testing.T) {
	// 设置监听端口，模拟外部服务器
	echoServer, err := net.Listen("tcp", "127.0.0.1:0") // 0 表示自动选择端口
	assert.NoError(t, err)
	defer echoServer.Close()

	msg := "Hello, TCP!"

	go func() {
		for {
			conn, err := echoServer.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c)
			}(conn)
		}
	}()

	clientConn, err := net.Dial("tcp", echoServer.Addr().String())
	assert.NoError(t, err)
	defer clientConn.Close()

	remoteConn, err := net.Dial("tcp", echoServer.Addr().String())
	assert.NoError(t, err)
	defer remoteConn.Close()
	testOptions := conf.Options{IdleTimeout: time.Second, ReadTimeout: time.Second}
	rc := relayConnImpl{Stats: &Stats{}, remote: &lb.Node{}, Options: &testOptions}
	c1 := newInnerConn(clientConn, &rc)
	c2 := newInnerConn(remoteConn, &rc)

	done := make(chan struct{})
	go func() {
		if err := copyConn(c1, c2, zap.S()); err != nil {
			t.Log(err)
		}
		done <- struct{}{}
		close(done)
	}()

	_, err = clientConn.Write([]byte(msg))
	assert.NoError(t, err)

	buffer := make([]byte, len(msg))
	_, err = clientConn.Read(buffer)
	assert.NoError(t, err)
	assert.Equal(t, msg, string(buffer))
	// close the connection
	_ = clientConn.Close()
	_ = remoteConn.Close()
	// wait for the copyConn to finish
	<-done
}

func TestCopyUDPConn(t *testing.T) {
	// 设置监听地址，模拟外部UDP服务器
	serverAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	assert.NoError(t, err)

	echoServer, err := net.ListenUDP("udp", serverAddr)
	assert.NoError(t, err)
	defer echoServer.Close()

	msg := "Hello, UDP!"

	go func() {
		buffer := make([]byte, 1024)
		for {
			n, remoteAddr, err := echoServer.ReadFromUDP(buffer)
			if err != nil {
				return
			}
			_, err = echoServer.WriteToUDP(buffer[:n], remoteAddr)
			if err != nil {
				return
			}
		}
	}()

	clientConn, err := net.DialUDP("udp", nil, echoServer.LocalAddr().(*net.UDPAddr))
	assert.NoError(t, err)
	defer clientConn.Close()

	remoteConn, err := net.DialUDP("udp", nil, echoServer.LocalAddr().(*net.UDPAddr))
	assert.NoError(t, err)
	defer remoteConn.Close()

	testOptions := conf.Options{IdleTimeout: time.Second, ReadTimeout: time.Second}
	rc := relayConnImpl{Stats: &Stats{}, remote: &lb.Node{}, Options: &testOptions}
	c1 := newInnerConn(clientConn, &rc)
	c2 := newInnerConn(remoteConn, &rc)

	done := make(chan struct{})
	go func() {
		if err := copyConn(c1, c2, zap.S()); err != nil {
			t.Log(err)
		}
		done <- struct{}{}
		close(done)
	}()

	_, err = clientConn.Write([]byte(msg))
	assert.NoError(t, err)

	buffer := make([]byte, len(msg))
	n, _, err := clientConn.ReadFromUDP(buffer)
	assert.NoError(t, err)
	assert.Equal(t, msg, string(buffer[:n]))

	// 关闭连接
	_ = clientConn.Close()
	_ = remoteConn.Close()

	// 等待 copyConn 完成
	<-done
}
