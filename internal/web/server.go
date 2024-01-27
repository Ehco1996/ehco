package web

import (
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"

	"github.com/Ehco1996/ehco/internal/config"
	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

type Server struct {
	e    *echo.Echo
	addr string
	l    *zap.SugaredLogger

	cfg *config.Config
}

func NewServer(cfg *config.Config) (*Server, error) {
	l := zap.S().Named("web")

	addr := net.JoinHostPort(cfg.WebHost, fmt.Sprintf("%d", cfg.WebPort))
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Use(NginxLogMiddleware(l))

	if cfg.WebToken != "" {
		e.Use(SimpleTokenAuthMiddleware(cfg.WebToken, l))
	}

	if err := registerEhcoMetrics(cfg); err != nil {
		return nil, err
	}
	if err := registerNodeExporterMetrics(cfg); err != nil {
		return nil, err
	}
	s := &Server{e: e, addr: addr, l: l, cfg: cfg}

	// register handler
	e.GET("/", echo.WrapHandler(http.HandlerFunc(welcome)))
	e.GET("/metrics/", echo.WrapHandler(promhttp.Handler()))
	e.GET("/debug/pprof/*", echo.WrapHandler(http.DefaultServeMux))
	e.GET("/clash_proxy_provider/", echo.WrapHandler(http.HandlerFunc(s.HandleClashProxyProvider)))
	return s, nil
}

func (s *Server) Start() error {
	s.l.Infof("Start Web Server at http://%s", s.addr)
	return s.e.Start(s.addr)
}

func (s *Server) Stop() error {
	return s.e.Close()
}
