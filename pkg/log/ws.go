package log

import (
	"encoding/json"
	"net"
	"sync"

	"github.com/gobwas/ws"
)

type WebSocketLogSyncher struct {
	conn net.Conn
	mu   sync.Mutex
}

func NewWebSocketLogSyncher() *WebSocketLogSyncher {
	return &WebSocketLogSyncher{}
}

func (wsSync *WebSocketLogSyncher) Write(p []byte) (n int, err error) {
	wsSync.mu.Lock()
	defer wsSync.mu.Unlock()

	if wsSync.conn != nil {
		var logEntry map[string]interface{}
		if err := json.Unmarshal(p, &logEntry); err == nil {
			jsonData, _ := json.Marshal(logEntry)
			_ = ws.WriteFrame(wsSync.conn, ws.NewTextFrame(jsonData))
		}

		if err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

func (wsSync *WebSocketLogSyncher) Sync() error {
	return nil
}

func (wsSync *WebSocketLogSyncher) SetWSConn(conn net.Conn) {
	wsSync.mu.Lock()
	defer wsSync.mu.Unlock()
	wsSync.conn = conn
}

func SetWebSocketConn(conn net.Conn) {
	if globalWebSocketSyncher != nil {
		globalWebSocketSyncher.SetWSConn(conn)
	}
}
