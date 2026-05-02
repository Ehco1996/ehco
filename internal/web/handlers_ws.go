package web

import (
	"net"

	"github.com/Ehco1996/ehco/pkg/log"
	"github.com/gobwas/ws"
	"github.com/labstack/echo/v4"
)

func (s *Server) handleWebSocketLogs(c echo.Context) error {
	conn, _, _, err := ws.UpgradeHTTP(c.Request(), c.Response())
	if err != nil {
		return err
	}
	defer conn.Close()

	log.AddWebSocketConn(conn)
	defer log.RemoveWebSocketConn(conn)

	for {
		_, err := ws.ReadFrame(conn)
		if err != nil {
			if _, ok := err.(net.Error); ok {
				s.l.Errorf("WebSocket read error: %v", err)
			}
			break
		}
	}
	return nil
}
