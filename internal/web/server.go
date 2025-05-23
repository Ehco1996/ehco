package web

import (
	"crypto/subtle"
	"embed"
	"fmt"
	"crypto/subtle"
	"embed"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	_ "net/http/pprof"
	"path"
	"strings"

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

//go:embed webui/frontend/dist
var embeddedUIDir embed.FS

//go:embed webui/frontend/dist/assets
var embeddedUIAssetsDir embed.FS

const (
	metricsPath = "/metrics/"
	apiPrefix   = "/api/v1"
	wsLogsPath  = "/ws/logs" // Added for clarity
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

	// setupTemplates is removed

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

// setupTemplates is removed

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

	// Serve static assets from webui/frontend/dist/assets
	// The path needs to be relative to the embedded directory structure
	assetsFS, err := fs.Sub(embeddedUIDir, "webui/frontend/dist/assets")
	if err != nil {
		s.l.Fatalf("failed to create sub FS for assets: %v", err)
	}
	e.StaticFS("/assets", assetsFS)

	// API routes
	api := e.Group(apiPrefix)
	api.GET("/config/", s.CurrentConfig)
	api.POST("/config/reload/", s.HandleReload)
	api.GET("/health_check/", s.HandleHealthCheck)
	api.GET("/node_metrics/", s.GetNodeMetrics)
	api.GET("/rule_metrics/", s.GetRuleMetrics)
	api.GET("/connections/", s.GetConnections)

	// WebSocket route
	e.GET(wsLogsPath, s.handleWebSocketLogs)

	// Prometheus metrics
	e.GET(metricsPath, echo.WrapHandler(promhttp.Handler()))
	// pprof
	e.GET("/debug/pprof/*", echo.WrapHandler(http.DefaultServeMux))

	// Catch-all for SPA: Serves index.html for any route not handled above
	// This allows React Router to handle client-side routing.
	e.GET("/*", func(c echo.Context) error {
		// Do not serve index.html for API calls, WS, or other specific server-handled paths
		// (though these should be matched by earlier routes)
		p := c.Request().URL.Path
		if strings.HasPrefix(p, apiPrefix) ||
			strings.HasPrefix(p, wsLogsPath) ||
			strings.HasPrefix(p, metricsPath) ||
			strings.HasPrefix(p, "/assets") || // Already handled by StaticFS
			strings.HasPrefix(p, "/debug/pprof") {
			// Let Echo handle it as 404 or specific handler
			return echo.NotFoundHandler(c)
		}

		indexPath := "webui/frontend/dist/index.html"
		fileBytes, err := embeddedUIDir.ReadFile(indexPath)
		if err != nil {
			// Try to read from subFS in case path is relative to it
			// This part might be tricky depending on how embed resolves paths
			content, err := fs.ReadFile(embeddedUIDir, path.Join("webui/frontend/dist", c.Param("*")))
			if err == nil {
				return c.HTMLBlob(http.StatusOK, content)
			}
			s.l.Errorf("SPA index.html not found or error reading: %v. Attempted path: %s", err, indexPath)
			return echo.NewHTTPError(http.StatusInternalServerError, "SPA index file not found")
		}
		return c.HTMLBlob(http.StatusOK, fileBytes)
	})
}

func (s *Server) Start() error {
	s.l.Infof("Start Web Server at http://%s", s.addr)
	return s.e.Start(s.addr)
}

func (s *Server) Stop() error {
	return s.e.Close()
}
