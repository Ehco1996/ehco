package log

import (
	"encoding/json"
	"net"
	"sync"

	"github.com/gobwas/ws"
)

// WebSocketLogSyncher fans out log frames to all currently-attached
// WebSocket subscribers. A subscriber that errors on write is removed
// so a single dead conn cannot stall the others.
type WebSocketLogSyncher struct {
	mu    sync.Mutex
	conns map[net.Conn]struct{}
}

func NewWebSocketLogSyncher() *WebSocketLogSyncher {
	return &WebSocketLogSyncher{conns: make(map[net.Conn]struct{})}
}

func (wsSync *WebSocketLogSyncher) Write(p []byte) (n int, err error) {
	wsSync.mu.Lock()
	if len(wsSync.conns) == 0 {
		wsSync.mu.Unlock()
		return len(p), nil
	}

	var logEntry map[string]interface{}
	if err := json.Unmarshal(p, &logEntry); err != nil {
		wsSync.mu.Unlock()
		return len(p), nil
	}
	jsonData, _ := json.Marshal(logEntry)
	frame := ws.NewTextFrame(jsonData)

	var dead []net.Conn
	for c := range wsSync.conns {
		if err := ws.WriteFrame(c, frame); err != nil {
			dead = append(dead, c)
		}
	}
	for _, c := range dead {
		delete(wsSync.conns, c)
	}
	wsSync.mu.Unlock()

	for _, c := range dead {
		_ = c.Close()
	}
	return len(p), nil
}

func (wsSync *WebSocketLogSyncher) Sync() error {
	return nil
}

func (wsSync *WebSocketLogSyncher) AddConn(conn net.Conn) {
	wsSync.mu.Lock()
	defer wsSync.mu.Unlock()
	wsSync.conns[conn] = struct{}{}
}

func (wsSync *WebSocketLogSyncher) RemoveConn(conn net.Conn) {
	wsSync.mu.Lock()
	defer wsSync.mu.Unlock()
	delete(wsSync.conns, conn)
}

// AddWebSocketConn attaches a WebSocket conn as a log subscriber.
func AddWebSocketConn(conn net.Conn) {
	if globalWebSocketSyncher != nil {
		globalWebSocketSyncher.AddConn(conn)
	}
}

// RemoveWebSocketConn detaches a previously-attached WebSocket conn.
func RemoveWebSocketConn(conn net.Conn) {
	if globalWebSocketSyncher != nil {
		globalWebSocketSyncher.RemoveConn(conn)
	}
}
