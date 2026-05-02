package web

import (
	"crypto/subtle"
	"fmt"
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

const (
	metricsPath = "/metrics/"
	apiPrefix   = "/api/v1"
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
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	l := zap.S().Named("web")

	e := NewEchoServer()
	if err := setupMiddleware(e, cfg, l); err != nil {
		return nil, fmt.Errorf("failed to setup middleware: %w", err)
	}

	if err := setupMetrics(cfg); err != nil {
		return nil, fmt.Errorf("failed to setup metrics: %w", err)
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
	if cfg.WebPort <= 0 || cfg.WebPort > 65535 {
		return fmt.Errorf("invalid web port: %d", cfg.WebPort)
	}
	return nil
}

func setupMiddleware(e *echo.Echo, cfg *config.Config, _ *zap.SugaredLogger) error {
	e.Use(NginxLogMiddleware(zap.S().Named("web")))

	skipPublic := func(c echo.Context) bool {
		// SPA shell + its content-hashed assets must load without a token,
		// otherwise the user can't reach the login modal to enter one.
		// API/metrics/ws/pprof keep their auth.
		return isPublicPath(c.Request().URL.Path)
	}

	if cfg.WebToken != "" {
		e.Use(middleware.KeyAuthWithConfig(middleware.KeyAuthConfig{
			Skipper:   skipPublic,
			KeyLookup: "query:token",
			Validator: func(key string, _ echo.Context) (bool, error) {
				return key == cfg.WebToken, nil
			},
			// Default behaviour returns 400 when the key is missing entirely;
			// flatten to 401 so the SPA can treat "needs auth" uniformly
			// regardless of whether the user submitted a wrong token or none.
			ErrorHandler: func(err error, _ echo.Context) error {
				return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
			},
		}))
	}

	if cfg.WebAuthUser != "" && cfg.WebAuthPass != "" {
		e.Use(middleware.BasicAuthWithConfig(middleware.BasicAuthConfig{
			Skipper: skipPublic,
			Validator: func(username, password string, _ echo.Context) (bool, error) {
				if subtle.ConstantTimeCompare([]byte(username), []byte(cfg.WebAuthUser)) == 1 &&
					subtle.ConstantTimeCompare([]byte(password), []byte(cfg.WebAuthPass)) == 1 {
					return true, nil
				}
				return false, nil
			},
		}))
	}

	return nil
}

func setupMetrics(cfg *config.Config) error {
	if err := metrics.RegisterEhcoMetrics(cfg); err != nil {
		return fmt.Errorf("failed to register Ehco metrics: %w", err)
	}
	if err := metrics.RegisterNodeExporterMetrics(cfg); err != nil {
		return fmt.Errorf("failed to register Node Exporter metrics: %w", err)
	}
	return nil
}

func setupRoutes(s *Server) {
	e := s.e

	e.GET(metricsPath, echo.WrapHandler(promhttp.Handler()))
	e.GET("/debug/pprof/*", echo.WrapHandler(http.DefaultServeMux))

	api := e.Group(apiPrefix)
	api.GET("/config/", s.CurrentConfig)
	api.POST("/config/reload/", s.HandleReload)
	api.GET("/health_check/", s.HandleHealthCheck)
	api.GET("/node_metrics/", s.GetNodeMetrics)
	api.GET("/rule_metrics/", s.GetRuleMetrics)

	e.GET("/ws/logs", s.handleWebSocketLogs)

	// SPA: assets are served from the embedded dist tree, every other
	// path falls through to the SPA shell so client-side routing works.
	e.GET("/assets/*", assetHandler())
	e.GET("/favicon.ico", assetHandler())
	e.GET("/", spaHandler())
	e.GET("/*", spaHandler())
}

// APIGroup returns the /api/v1 echo group so other components (e.g. XrayServer)
// can mount their own endpoints under the same auth/middleware stack.
// Must be called before Start.
func (s *Server) APIGroup() *echo.Group {
	return s.e.Group(apiPrefix)
}

func (s *Server) Start() error {
	s.l.Infof("Start Web Server at http://%s", s.addr)
	return s.e.Start(s.addr)
}

func (s *Server) Stop() error {
	return s.e.Close()
}
