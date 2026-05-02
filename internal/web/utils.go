package web

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

func NewEchoServer() *echo.Echo {
	e := echo.New()
	e.Debug = true
	e.HidePort = true
	e.HideBanner = true
	return e
}

// MakeIndexF returns a tiny "/" handler used by the relay WS server and
// xray's debug listener to surface the caller's IP. Kept here (rather than
// in the SPA admin server) so transports that don't run the full admin
// server can still use it.
func MakeIndexF() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		zap.S().Named("web").Infof("index call from %s", r.RemoteAddr)
		_, _ = fmt.Fprintf(w, "access from remote ip: %s \n", r.RemoteAddr)
	}
}
