package web

import (
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/Ehco1996/ehco/internal/cmgr"
	"github.com/Ehco1996/ehco/internal/config"
	"github.com/Ehco1996/ehco/internal/metrics"
	"github.com/Ehco1996/ehco/internal/reloader"
)

type Server struct {
	e    *echo.Echo
	addr string
	l    *zap.SugaredLogger
	cfg  *config.Config

	relayServerReloader reloader.Reloader
	connMgr             cmgr.Cmgr
}

func NewServer(cfg *config.Config, relayReloader reloader.Reloader, connMgr cmgr.Cmgr) (*Server, error) {
	l := zap.S().Named("web")

	addr := net.JoinHostPort(cfg.WebHost, fmt.Sprintf("%d", cfg.WebPort))
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Use(NginxLogMiddleware(l))

	if cfg.WebToken != "" {
		e.Use(SimpleTokenAuthMiddleware(cfg.WebToken, l))
	}
	if err := metrics.RegisterEhcoMetrics(cfg); err != nil {
		return nil, err
	}
	if err := metrics.RegisterNodeExporterMetrics(cfg); err != nil {
		return nil, err
	}
	s := &Server{
		e:                   e,
		addr:                addr,
		l:                   l,
		cfg:                 cfg,
		relayServerReloader: relayReloader,
		connMgr:             connMgr,
	}

	// register handler
	e.GET("/", echo.WrapHandler(http.HandlerFunc(s.welcome)))
	e.GET("/metrics/", echo.WrapHandler(promhttp.Handler()))
	e.GET("/debug/pprof/*", echo.WrapHandler(http.DefaultServeMux))
	e.GET("/clash_proxy_provider/", echo.WrapHandler(http.HandlerFunc(s.HandleClashProxyProvider)))
	e.GET("/config/", echo.WrapHandler(http.HandlerFunc(s.CurrentConfig)))

	e.POST("/reload/", echo.WrapHandler(http.HandlerFunc(s.HandleReload)))

	// api group
	api := e.Group("/api/v1")
	api.GET("/connections/", s.ListConnections)

	return s, nil
}

func (s *Server) Start() error {
	s.l.Infof("Start Web Server at http://%s", s.addr)
	return s.e.Start(s.addr)
}

func (s *Server) Stop() error {
	return s.e.Close()
}
