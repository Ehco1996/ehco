package web

import (
	"crypto/subtle"
	"embed"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	_ "net/http/pprof"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/Ehco1996/ehco/internal/cmgr"
	"github.com/Ehco1996/ehco/internal/config"
	"github.com/Ehco1996/ehco/internal/glue"
	"github.com/Ehco1996/ehco/internal/metrics"
)

//go:embed templates/*.html
var templatesFS embed.FS

type Server struct {
	glue.Reloader
	glue.HealthChecker

	e    *echo.Echo
	addr string
	l    *zap.SugaredLogger
	cfg  *config.Config

	connMgr cmgr.Cmgr
}

type echoTemplate struct {
	templates *template.Template
}

func (t *echoTemplate) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

func NewServer(
	cfg *config.Config,
	relayReloader glue.Reloader,
	healthChecker glue.HealthChecker,
	connMgr cmgr.Cmgr,
) (*Server, error) {
	l := zap.S().Named("web")

	templates := template.Must(template.ParseFS(templatesFS, "templates/*.html"))
	for _, temp := range templates.Templates() {
		l.Debug("template name: ", temp.Name())
	}
	e := NewEchoServer()
	e.Use(NginxLogMiddleware(l))
	e.Renderer = &echoTemplate{templates: templates}
	if cfg.WebToken != "" {
		e.Use(middleware.KeyAuthWithConfig(middleware.KeyAuthConfig{
			KeyLookup: "query:token",
			Validator: func(key string, c echo.Context) (bool, error) {
				return key == cfg.WebToken, nil
			},
		}))
	}

	if cfg.WebAuthUser != "" && cfg.WebAuthPass != "" {
		e.Use(middleware.BasicAuth(func(username, password string, c echo.Context) (bool, error) {
			// Be careful to use constant time comparison to prevent timing attacks
			if subtle.ConstantTimeCompare([]byte(username), []byte(cfg.WebAuthUser)) == 1 &&
				subtle.ConstantTimeCompare([]byte(password), []byte(cfg.WebAuthPass)) == 1 {
				return true, nil
			}
			return false, nil
		}))
	}

	if err := metrics.RegisterEhcoMetrics(cfg); err != nil {
		return nil, err
	}
	if err := metrics.RegisterNodeExporterMetrics(cfg); err != nil {
		return nil, err
	}
	s := &Server{
		Reloader:      relayReloader,
		HealthChecker: healthChecker,

		e:       e,
		l:       l,
		cfg:     cfg,
		connMgr: connMgr,
		addr:    net.JoinHostPort(cfg.WebHost, fmt.Sprintf("%d", cfg.WebPort)),
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
	api.GET("/health_check/", s.HandleHealthCheck)
	return s, nil
}

func (s *Server) Start() error {
	s.l.Infof("Start Web Server at http://%s", s.addr)
	return s.e.Start(s.addr)
}

func (s *Server) Stop() error {
	return s.e.Close()
}
