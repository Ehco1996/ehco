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
	// 设置一个小的超时时长，以便在没有数据交换时测试可以快速失败而不是永久阻塞
	clientConn.SetDeadline(time.Now().Add(1 * time.Second))
	serverConn.SetDeadline(time.Now().Add(1 * time.Second))
	defer clientConn.Close()
	defer serverConn.Close()

	innerC := &innerConn{Conn: clientConn, stats: &Stats{}, remoteLabel: "test"}

	errChan := make(chan error, 1) // 用于从 goroutine 向主流程报告错误

	// 测试 innerConn.Write
	go func() {
		_, err := innerC.Write(testData)
		errChan <- err // 将错误发送回主流程
	}()

	buf := make([]byte, len(testData))
	n, err := serverConn.Read(buf)
	if err != nil {
		t.Fatalf("读操作失败: %v", err)
	}
	assert.Equal(t, n, len(testData))
	assert.Equal(t, testData, buf)

	// 检查写操作是否出错 -- 这里的关键是，在检查前确定写操作已经完成
	if err := <-errChan; err != nil {
		t.Fatalf("写操作失败: %v", err)
	}
	// 由于此时已确定写操作完成，可以安全地检查stats
	assert.Equal(t, int64(len(testData)), innerC.stats.Up)

	// 重置错误通道和超时，准备测试 Read
	errChan = make(chan error, 1)
	clientConn.SetDeadline(time.Now().Add(1 * time.Second))
	serverConn.SetDeadline(time.Now().Add(1 * time.Second))

	// 测试 innerConn.Read
	go func() {
		_, err := serverConn.Write(testData)
		errChan <- err // 将错误发送回主流程
	}()

	n, err = innerC.Read(buf)
	if err != nil {
		if errors.Is(err, io.EOF) {
			t.Logf("读取到 EOF，可能是连接关闭")
		} else {
			t.Fatalf("读操作失败: %v", err)
		}
	}
	assert.Equal(t, n, len(testData))
	assert.Equal(t, testData, buf)

	// 检查写操作是否出错
	if err := <-errChan; err != nil {
		t.Fatalf("写操作失败: %v", err)
	}
	assert.Equal(t, int64(len(testData)), innerC.stats.Down)
}
