package web

import (
	"embed"
	"fmt"
	"html/template"
	"io"
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

//go:embed templates/*.html
var templatesFS embed.FS

type Server struct {
	e    *echo.Echo
	addr string
	l    *zap.SugaredLogger
	cfg  *config.Config

	relayServerReloader reloader.Reloader
	connMgr             cmgr.Cmgr
}

type echoTemplate struct {
	templates *template.Template
}

func (t *echoTemplate) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

func NewServer(cfg *config.Config, relayReloader reloader.Reloader, connMgr cmgr.Cmgr) (*Server, error) {
	l := zap.S().Named("web")

	templates := template.Must(template.ParseFS(templatesFS, "templates/*.html"))
	for _, temp := range templates.Templates() {
		l.Debug("template name: ", temp.Name())
	}
	e := echo.New()
	e.Debug = true
	e.HidePort = true
	e.HideBanner = true
	e.Use(NginxLogMiddleware(l))
	e.Renderer = &echoTemplate{templates: templates}

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
		l:                   l,
		cfg:                 cfg,
		connMgr:             connMgr,
		relayServerReloader: relayReloader,
		addr:                net.JoinHostPort(cfg.WebHost, fmt.Sprintf("%d", cfg.WebPort)),
	}

	// register handler
	e.GET("/metrics/", echo.WrapHandler(promhttp.Handler()))
	e.GET("/debug/pprof/*", echo.WrapHandler(http.DefaultServeMux))

	e.GET("/", s.index)
	e.GET("/connections/", s.ListConnections)
	e.GET("/clash_proxy_provider/", s.HandleClashProxyProvider)

	// api group
	api := e.Group("/api/v1")
	api.GET("/config/", s.CurrentConfig)
	api.POST("/config/reload/", s.HandleReload)

	return s, nil
}

func (s *Server) Start() error {
	s.l.Infof("Start Web Server at http://%s", s.addr)
	return s.e.Start(s.addr)
}

func (s *Server) Stop() error {
	return s.e.Close()
}
