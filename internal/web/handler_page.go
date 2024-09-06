package web

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/Ehco1996/ehco/internal/config"
	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

const defaultPageSize = 20

func MakeIndexF() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		zap.S().Named("web").Infof("index call from %s", r.RemoteAddr)
		fmt.Fprintf(w, "access from remote ip: %s \n", r.RemoteAddr)
	}
}

func (s *Server) index(c echo.Context) error {
	data := struct {
		Version     string
		GitBranch   string
		GitRevision string
		BuildTime   string
		StartTime   string
		Cfg         config.Config
	}{
		Version:     constant.Version,
		GitBranch:   constant.GitBranch,
		GitRevision: constant.GitRevision,
		BuildTime:   constant.BuildTime,
		StartTime:   constant.StartTime.Format("2006-01-02 15:04:05"),
		Cfg:         *s.cfg,
	}
	return c.Render(http.StatusOK, "index.html", data)
}

func (s *Server) ListConnections(c echo.Context) error {
	pageStr := c.QueryParam("page")
	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}
	pageSizeStr := c.QueryParam("page_size")
	pageSize, err := strconv.Atoi(pageSizeStr)
	if err != nil || pageSize < 1 {
		pageSize = defaultPageSize
	}
	connType := c.QueryParam("conn_type")
	total := s.connMgr.CountConnection(connType)
	perv := 0
	if page > 1 {
		perv = page - 1
	}
	next := 0
	if page*pageSize < total && page*pageSize > 0 {
		next = page + 1
	}

	activeCount := s.connMgr.CountConnection("active")
	closedCount := s.connMgr.CountConnection("closed")

	return c.Render(http.StatusOK, "connection.html", map[string]interface{}{
		"ConnType":       connType,
		"ConnectionList": s.connMgr.ListConnections(connType, page, pageSize),
		"CurrentPage":    page,
		"TotalPage":      total / pageSize,
		"PageSize":       pageSize,
		"Prev":           perv,
		"Next":           next,
		"Count":          total,
		"ActiveCount":    activeCount,
		"ClosedCount":    closedCount,
		"AllCount":       s.connMgr.CountConnection("active") + s.connMgr.CountConnection("closed"),
	})
}

func (s *Server) ListRules(c echo.Context) error {
	return c.Render(http.StatusOK, "rule_list.html", map[string]interface{}{
		"Configs": s.cfg.RelayConfigs,
	})
}

func (s *Server) RuleMetrics(c echo.Context) error {
	return c.Render(http.StatusOK, "rule_metrics.html", map[string]interface{}{
		"Configs": s.cfg.RelayConfigs,
	})
}

func (s *Server) LogsPage(c echo.Context) error {
	return c.Render(http.StatusOK, "logs.html", nil)
}
