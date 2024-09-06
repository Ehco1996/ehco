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
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/Ehco1996/ehco/internal/cmgr"
	"github.com/Ehco1996/ehco/internal/config"
	"github.com/Ehco1996/ehco/internal/glue"
	"github.com/Ehco1996/ehco/internal/metrics"
)

//go:embed templates/*.html js/*.js
var templatesFS embed.FS

const (
	metricsPath     = "/metrics/"
	indexPath       = "/"
	connectionsPath = "/connections/"
	rulesPath       = "/rules/"
	apiPrefix       = "/api/v1"
)

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
	if err := validateConfig(cfg); err != nil {
		return nil, errors.Wrap(err, "invalid configuration")
	}

	l := zap.S().Named("web")

	e := NewEchoServer()
	if err := setupMiddleware(e, cfg, l); err != nil {
		return nil, errors.Wrap(err, "failed to setup middleware")
	}

	if err := setupTemplates(e, l, cfg); err != nil {
		return nil, errors.Wrap(err, "failed to setup templates")
	}

	if err := setupMetrics(cfg); err != nil {
		return nil, errors.Wrap(err, "failed to setup metrics")
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

	setupRoutes(s)

	return s, nil
}

func validateConfig(cfg *config.Config) error {
	// Add validation logic here
	if cfg.WebPort <= 0 || cfg.WebPort > 65535 {
		return errors.New("invalid web port")
	}
	// Add more validations as needed
	return nil
}

func setupMiddleware(e *echo.Echo, cfg *config.Config, l *zap.SugaredLogger) error {
	e.Use(NginxLogMiddleware(l))

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
			if subtle.ConstantTimeCompare([]byte(username), []byte(cfg.WebAuthUser)) == 1 &&
				subtle.ConstantTimeCompare([]byte(password), []byte(cfg.WebAuthPass)) == 1 {
				return true, nil
			}
			return false, nil
		}))
	}

	return nil
}

func setupTemplates(e *echo.Echo, l *zap.SugaredLogger, cfg *config.Config) error {
	funcMap := template.FuncMap{
		"sub": func(a, b int) int { return a - b },
		"add": func(a, b int) int { return a + b },
		"CurrentCfg": func() *config.Config {
			return cfg
		},
	}
	tmpl, err := template.New("").Funcs(funcMap).ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return errors.Wrap(err, "failed to parse templates")
	}
	templates := template.Must(tmpl, nil)
	for _, temp := range templates.Templates() {
		l.Debug("template name: ", temp.Name())
	}
	e.Renderer = &echoTemplate{templates: templates}
	return nil
}

func setupMetrics(cfg *config.Config) error {
	if err := metrics.RegisterEhcoMetrics(cfg); err != nil {
		return errors.Wrap(err, "failed to register Ehco metrics")
	}
	if err := metrics.RegisterNodeExporterMetrics(cfg); err != nil {
		return errors.Wrap(err, "failed to register Node Exporter metrics")
	}
	return nil
}

func setupRoutes(s *Server) {
	e := s.e

	e.StaticFS("/js", echo.MustSubFS(templatesFS, "js"))
	e.GET(metricsPath, echo.WrapHandler(promhttp.Handler()))
	e.GET("/debug/pprof/*", echo.WrapHandler(http.DefaultServeMux))

	// web pages
	e.GET(indexPath, s.index)
	e.GET(connectionsPath, s.ListConnections)
	e.GET(rulesPath, s.ListRules)
	e.GET("/rule_metrics/", s.RuleMetrics)
	e.GET("/logs/", s.LogsPage)

	api := e.Group(apiPrefix)
	api.GET("/config/", s.CurrentConfig)
	api.POST("/config/reload/", s.HandleReload)
	api.GET("/health_check/", s.HandleHealthCheck)
	api.GET("/node_metrics/", s.GetNodeMetrics)
	api.GET("/rule_metrics/", s.GetRuleMetrics)

	// ws
	e.GET("/ws/logs", s.handleWebSocketLogs)
}

func (s *Server) Start() error {
	s.l.Infof("Start Web Server at http://%s", s.addr)
	return s.e.Start(s.addr)
}

func (s *Server) Stop() error {
	return s.e.Close()
}
