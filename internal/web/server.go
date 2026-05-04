package web

import (
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"sync/atomic"

	"github.com/labstack/echo/v4"
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
	auth *authenticator

	connMgr   cmgr.Cmgr
	updateJob atomic.Pointer[JobStatus]

	// xrayStatus is wired post-construction by cli boot once the
	// XrayServer exists. Always read via Load() — may be nil when
	// xray sync is disabled. Atomic pointer keeps it lock-free.
	xrayStatus atomic.Pointer[glue.XrayStatus]
}

// SetXrayStatus is called by cli boot once the XrayServer is
// constructed. The /overview handler picks it up via Load().
func (s *Server) SetXrayStatus(p glue.XrayStatus) {
	if p == nil {
		return
	}
	s.xrayStatus.Store(&p)
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
	auth := newAuthenticator(cfg)
	setupMiddleware(e, auth)

	if err := setupMetrics(cfg); err != nil {
		return nil, fmt.Errorf("failed to setup metrics: %w", err)
	}

	s := &Server{
		Reloader:      relayReloader,
		HealthChecker: healthChecker,

		e:       e,
		l:       l,
		cfg:     cfg,
		auth:    auth,
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

func setupMiddleware(e *echo.Echo, auth *authenticator) {
	e.Use(NginxLogMiddleware(zap.S().Named("web")))
	// Single auth middleware: cookie-session for browsers, bearer-header
	// for machine clients. No more dual gate, no more query-string token.
	e.Use(auth.authMiddleware())
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
	api.GET("/auth/info", s.AuthInfo)
	api.POST("/auth/login", s.HandleLogin)
	api.POST("/auth/logout", s.HandleLogout)
	api.GET("/config/", s.CurrentConfig)
	api.POST("/config/reload/", s.HandleReload)
	api.GET("/health_check/", s.HandleHealthCheck)
	api.GET("/node_metrics/", s.GetNodeMetrics)
	api.GET("/rule_metrics/", s.GetRuleMetrics)
	api.GET("/overview", s.Overview)
	api.GET("/version", s.Version)
	api.GET("/update/check", s.UpdateCheck)
	api.POST("/update/apply", s.UpdateApply)
	api.GET("/update/status", s.UpdateStatus)

	// Local SQLite store: read-side health snapshot + maintenance ops.
	// All four mutations are auth-gated through the api group's
	// existing middleware — the /db/truncate confirm-string is a
	// second line of defence, not a first.
	api.GET("/db/health", s.GetDBHealth)
	api.POST("/db/cleanup", s.PostDBCleanup)
	api.POST("/db/vacuum", s.PostDBVacuum)
	api.POST("/db/truncate", s.PostDBTruncate)
	api.POST("/db/reset_stats", s.PostDBResetStats)

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
