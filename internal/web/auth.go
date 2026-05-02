package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/Ehco1996/ehco/internal/config"
)

const (
	sessionCookieName = "ehco_sid"
	sessionTTL        = 7 * 24 * time.Hour
	bearerHeader      = "Authorization"
	bearerPrefix      = "Bearer "
	apiTokenHeader    = "X-Ehco-Token"
)

// sessionStore is an in-memory sid → expiry map. We accept loss-on-restart
// because the dashboard is a low-frequency tool and the fleet is small;
// users re-login after a binary upgrade. No persistence layer (sqlite,
// Redis, …) is worth the operational weight at this scale.
type sessionStore struct {
	mu       sync.Mutex
	sessions map[string]time.Time
}

func newSessionStore() *sessionStore {
	return &sessionStore{sessions: make(map[string]time.Time)}
}

func (s *sessionStore) issue() (string, time.Time, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", time.Time{}, err
	}
	sid := base64.RawURLEncoding.EncodeToString(buf)
	expiry := time.Now().Add(sessionTTL)
	s.mu.Lock()
	s.sessions[sid] = expiry
	s.mu.Unlock()
	return sid, expiry, nil
}

// validate returns (ok, newExpiry). Sliding refresh: every successful
// hit pushes the expiry back to now+TTL so an actively used session
// doesn't expire while the user is on the page.
func (s *sessionStore) validate(sid string) (bool, time.Time) {
	if sid == "" {
		return false, time.Time{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.sessions[sid]
	if !ok {
		return false, time.Time{}
	}
	now := time.Now()
	if now.After(exp) {
		delete(s.sessions, sid)
		return false, time.Time{}
	}
	newExp := now.Add(sessionTTL)
	s.sessions[sid] = newExp
	return true, newExp
}

func (s *sessionStore) revoke(sid string) {
	if sid == "" {
		return
	}
	s.mu.Lock()
	delete(s.sessions, sid)
	s.mu.Unlock()
}

// authenticator combines the session store with config-driven bearer-token
// validation. One type owns both because the middleware needs to ask
// "is this request authenticated?" and the answer can come from either side.
type authenticator struct {
	cfg      *config.Config
	sessions *sessionStore
	l        *zap.SugaredLogger
}

func newAuthenticator(cfg *config.Config) *authenticator {
	return &authenticator{
		cfg:      cfg,
		sessions: newSessionStore(),
		l:        zap.S().Named("auth"),
	}
}

// authRequired reports whether dashboard auth is configured at all.
// When false (no DashboardPass, no ApiToken), the dashboard is fully
// open — used in dev / fresh deployments where credentials haven't
// been pushed yet.
func (a *authenticator) authRequired() bool {
	return a.cfg.DashboardPass != "" || a.cfg.ApiToken != ""
}

// checkRequest tries cookie session, then bearer header, then x-ehco-token.
// Returns (authenticated, sidToRefresh). If sidToRefresh is non-empty
// the middleware should re-issue the cookie with the slid expiry.
func (a *authenticator) checkRequest(r *http.Request) (bool, string, time.Time) {
	if !a.authRequired() {
		return true, "", time.Time{}
	}

	// 1. Cookie — preferred for browsers.
	if c, err := r.Cookie(sessionCookieName); err == nil {
		if ok, exp := a.sessions.validate(c.Value); ok {
			return true, c.Value, exp
		}
	}

	// 2. Bearer header — preferred for machine clients.
	if a.cfg.ApiToken != "" {
		if h := r.Header.Get(bearerHeader); strings.HasPrefix(h, bearerPrefix) {
			supplied := strings.TrimPrefix(h, bearerPrefix)
			if subtle.ConstantTimeCompare([]byte(supplied), []byte(a.cfg.ApiToken)) == 1 {
				return true, "", time.Time{}
			}
		}
		if h := r.Header.Get(apiTokenHeader); h != "" {
			if subtle.ConstantTimeCompare([]byte(h), []byte(a.cfg.ApiToken)) == 1 {
				return true, "", time.Time{}
			}
		}
	}

	return false, "", time.Time{}
}

// authMiddleware enforces the auth rules globally except for paths
// listed in isPublicPath. When auth is not configured the middleware
// is a no-op pass-through.
func (a *authenticator) authMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if isPublicPath(c.Request().URL.Path) {
				return next(c)
			}
			ok, sid, exp := a.checkRequest(c.Request())
			if !ok {
				return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
			}
			if sid != "" {
				// Slide cookie expiry. Keeps long-living dashboard tabs alive.
				setSessionCookie(c, sid, exp)
			}
			return next(c)
		}
	}
}

// setSessionCookie writes the sid cookie with attributes that match the
// connection. Secure is conditional on the request being over TLS; we
// can't unconditionally set it because the dashboard is also reachable
// over plain HTTP on the tailnet.
func setSessionCookie(c echo.Context, sid string, expiry time.Time) {
	cookie := &http.Cookie{
		Name:     sessionCookieName,
		Value:    sid,
		Path:     "/",
		Expires:  expiry,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   c.IsTLS(),
	}
	c.SetCookie(cookie)
}

func clearSessionCookie(c echo.Context) {
	cookie := &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   c.IsTLS(),
	}
	c.SetCookie(cookie)
}

// loginRequest is the body of POST /api/v1/auth/login. We ignore any
// "user" field a client might send — single-tenant, password only.
type loginRequest struct {
	Password string `json:"password"`
}

// HandleLogin authenticates against cfg.DashboardPass and, on success,
// issues a session cookie. Public endpoint.
func (s *Server) HandleLogin(c echo.Context) error {
	if !s.auth.authRequired() {
		// No password configured — login is meaningless. Don't 401, but
		// don't issue a session either; the SPA already knows from
		// /auth/info that nothing's required.
		return c.JSON(http.StatusOK, map[string]any{"authenticated": true})
	}
	if s.cfg.DashboardPass == "" {
		// ApiToken is set but DashboardPass isn't — machine-only deployment.
		// Browsers can't log in.
		return echo.NewHTTPError(http.StatusForbidden, "dashboard login not configured")
	}
	var req loginRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid login body")
	}
	if subtle.ConstantTimeCompare([]byte(req.Password), []byte(s.cfg.DashboardPass)) != 1 {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid password")
	}
	sid, exp, err := s.auth.sessions.issue()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to issue session")
	}
	setSessionCookie(c, sid, exp)
	return c.JSON(http.StatusOK, map[string]any{"authenticated": true})
}

// HandleLogout revokes the session and clears the cookie. Idempotent —
// safe to call from any state. Public endpoint so the SPA can sign out
// even if the cookie was somehow lost.
func (s *Server) HandleLogout(c echo.Context) error {
	if cookie, err := c.Request().Cookie(sessionCookieName); err == nil {
		s.auth.sessions.revoke(cookie.Value)
	}
	clearSessionCookie(c)
	return c.JSON(http.StatusOK, map[string]any{"authenticated": false})
}
