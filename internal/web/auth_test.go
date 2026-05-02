package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/Ehco1996/ehco/internal/config"
)

// helper: minimal echo + auth wired up, no dashboard routes. The test
// targets the middleware in isolation rather than the full Server,
// because the full Server pulls in xray/relay machinery we don't need
// here.
func mwHarness(t *testing.T, cfg *config.Config) (*echo.Echo, *authenticator) {
	t.Helper()
	e := echo.New()
	auth := newAuthenticator(cfg)
	e.Use(auth.authMiddleware())
	e.GET("/api/v1/config/", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})
	e.GET("/api/v1/auth/info", func(c echo.Context) error {
		// Mirror Server.AuthInfo's contract.
		ok, _, _ := auth.checkRequest(c.Request())
		return c.JSON(http.StatusOK, map[string]bool{
			"auth_required": auth.authRequired(),
			"authenticated": ok,
		})
	})
	return e, auth
}

func TestAuthDisabled_PassesThrough(t *testing.T) {
	e, _ := mwHarness(t, &config.Config{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config/", nil)
	rr := httptest.NewRecorder()
	e.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("auth disabled: expected 200, got %d body=%q", rr.Code, rr.Body.String())
	}
}

func TestAuthRequired_RejectsAnonymous(t *testing.T) {
	e, _ := mwHarness(t, &config.Config{DashboardPass: "letmein"})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config/", nil)
	rr := httptest.NewRecorder()
	e.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: expected 401, got %d", rr.Code)
	}
}

func TestAuthRequired_AcceptsValidSession(t *testing.T) {
	e, auth := mwHarness(t, &config.Config{DashboardPass: "letmein"})
	sid, _, err := auth.sessions.issue()
	if err != nil {
		t.Fatalf("issue session: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sid})
	rr := httptest.NewRecorder()
	e.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("valid session: expected 200, got %d body=%q", rr.Code, rr.Body.String())
	}
}

func TestAuthRequired_RejectsUnknownSid(t *testing.T) {
	e, _ := mwHarness(t, &config.Config{DashboardPass: "letmein"})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "not-a-real-sid"})
	rr := httptest.NewRecorder()
	e.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("unknown sid: expected 401, got %d", rr.Code)
	}
}

func TestAuthRequired_AcceptsBearer(t *testing.T) {
	e, _ := mwHarness(t, &config.Config{ApiToken: "secret-token"})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config/", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	rr := httptest.NewRecorder()
	e.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("bearer: expected 200, got %d", rr.Code)
	}
}

func TestAuthRequired_AcceptsXEhcoToken(t *testing.T) {
	e, _ := mwHarness(t, &config.Config{ApiToken: "secret-token"})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config/", nil)
	req.Header.Set("X-Ehco-Token", "secret-token")
	rr := httptest.NewRecorder()
	e.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("x-ehco-token: expected 200, got %d", rr.Code)
	}
}

func TestAuthRequired_RejectsWrongBearer(t *testing.T) {
	e, _ := mwHarness(t, &config.Config{ApiToken: "secret-token"})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config/", nil)
	req.Header.Set("Authorization", "Bearer something-else")
	rr := httptest.NewRecorder()
	e.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("wrong bearer: expected 401, got %d", rr.Code)
	}
}

func TestAuthInfo_PublicEvenWithAuthRequired(t *testing.T) {
	e, _ := mwHarness(t, &config.Config{DashboardPass: "letmein"})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/info", nil)
	rr := httptest.NewRecorder()
	e.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("auth/info should be public, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"auth_required":true`) {
		t.Fatalf("auth/info body missing auth_required=true: %q", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"authenticated":false`) {
		t.Fatalf("auth/info body missing authenticated=false: %q", rr.Body.String())
	}
}

func TestSessionStore_Sliding(t *testing.T) {
	s := newSessionStore()
	sid, exp1, err := s.issue()
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	ok, exp2 := s.validate(sid)
	if !ok {
		t.Fatalf("validate(fresh): expected ok")
	}
	// Sliding refresh should push expiry forward (or leave equal).
	if exp2.Before(exp1) {
		t.Fatalf("sliding refresh moved expiry backwards: %v < %v", exp2, exp1)
	}
}

func TestSessionStore_Revoke(t *testing.T) {
	s := newSessionStore()
	sid, _, _ := s.issue()
	s.revoke(sid)
	if ok, _ := s.validate(sid); ok {
		t.Fatalf("revoked sid should not validate")
	}
}

func TestLegacyConfigFolding(t *testing.T) {
	cfg := &config.Config{
		WebHost:     "127.0.0.1",
		WebPort:     9999,
		WebToken:    "legacy-token",
		WebAuthPass: "legacy-pass",
	}
	if err := cfg.Adjust(); err != nil {
		t.Fatalf("adjust: %v", err)
	}
	if cfg.DashboardPass != "legacy-pass" {
		t.Fatalf("WebAuthPass should fold to DashboardPass, got %q", cfg.DashboardPass)
	}
	if cfg.ApiToken != "legacy-token" {
		t.Fatalf("WebToken should fold to ApiToken, got %q", cfg.ApiToken)
	}
}
