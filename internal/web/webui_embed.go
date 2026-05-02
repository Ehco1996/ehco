package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

//go:embed all:webui/dist
var uiFS embed.FS

// uiSubFS is the file tree rooted at webui/dist, used both for the
// SPA-fallback handler and for serving content-hashed assets.
func uiSubFS() fs.FS {
	sub, err := fs.Sub(uiFS, "webui/dist")
	if err != nil {
		// `embed` validates the directive at compile time; this can only
		// happen if the dist dir is missing entirely, which means
		// `make ui` wasn't run.
		panic("webui/dist not embedded: run `make ui` before building")
	}
	return sub
}

// indexHTML returns the SPA shell. Cached at startup; small enough
// (<1KB) that the read cost is negligible.
func indexHTML() ([]byte, error) {
	return fs.ReadFile(uiSubFS(), "index.html")
}

// spaHandler serves either a real file from the embedded dist tree (for
// public assets like favicon.svg, robots.txt, anything Vite copies from
// /public) or the SPA shell when the path doesn't match a real file.
// Reserved prefixes (api/ws/metrics/debug) get a 404 so an unhandled API
// route doesn't get rewritten to HTML.
//
// Content-hashed assets under /assets/* are handled by assetHandler
// directly because they always have a matching file and we want the
// http.FileServer's cache headers.
func spaHandler() echo.HandlerFunc {
	shell, err := indexHTML()
	if err != nil {
		panic(err)
	}
	fsys := uiSubFS()
	fileSrv := http.FileServer(http.FS(fsys))
	return func(c echo.Context) error {
		p := c.Request().URL.Path
		for _, prefix := range []string{"/api/", "/ws/", "/metrics/", "/debug/"} {
			if strings.HasPrefix(p, prefix) {
				return echo.NewHTTPError(http.StatusNotFound)
			}
		}
		// If a real file exists at this path (favicon.svg, robots.txt,
		// anything dropped into webui/public/), serve it directly.
		clean := strings.TrimPrefix(p, "/")
		if clean != "" && clean != "index.html" {
			if f, err := fsys.Open(clean); err == nil {
				_ = f.Close()
				fileSrv.ServeHTTP(c.Response(), c.Request())
				return nil
			}
		}
		// Otherwise it's a client-side route — return the SPA shell.
		c.Response().Header().Set("Cache-Control", "no-store")
		return c.Blob(http.StatusOK, "text/html; charset=utf-8", shell)
	}
}

// assetHandler serves content-hashed bundles under /assets/* directly
// from the embedded dist tree. Returns 404 if the file doesn't exist so
// the SPA shell isn't accidentally returned for /assets/foo.
func assetHandler() echo.HandlerFunc {
	fsys := uiSubFS()
	srv := http.FileServer(http.FS(fsys))
	return func(c echo.Context) error {
		p := strings.TrimPrefix(c.Request().URL.Path, "/")
		f, err := fsys.Open(p)
		if err != nil {
			return echo.NewHTTPError(http.StatusNotFound)
		}
		_ = f.Close()
		srv.ServeHTTP(c.Response(), c.Request())
		return nil
	}
}

// isPublicPath identifies routes that should bypass the auth middleware
// so the SPA shell + its public assets and the login flow can load
// without a session. Metrics, config, WS, and pprof remain protected.
func isPublicPath(path string) bool {
	switch path {
	case "/", "/index.html", "/favicon.ico", "/favicon.svg", "/robots.txt":
		return true
	case "/api/v1/auth/info":
		// SPA queries this before login to know whether auth is required.
		return true
	case "/api/v1/auth/login", "/api/v1/auth/logout":
		// Login obviously needs to be reachable without a session;
		// logout is idempotent and safe to expose unauthenticated so
		// the SPA can clean up without a round-trip dance.
		return true
	}
	return strings.HasPrefix(path, "/assets/")
}
