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
