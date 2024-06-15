package web

import (
	"encoding/json"
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
		SubConfigs  []*config.SubConfig
	}{
		Version:     constant.Version,
		GitBranch:   constant.GitBranch,
		GitRevision: constant.GitRevision,
		BuildTime:   constant.BuildTime,
		StartTime:   constant.StartTime.Format("2006-01-02 15:04:05"),
		SubConfigs:  s.cfg.SubConfigs,
	}
	return c.Render(http.StatusOK, "index.html", data)
}

func (s *Server) HandleClashProxyProvider(c echo.Context) error {
	subName := c.QueryParam("sub_name")
	if subName == "" {
		return c.String(http.StatusBadRequest, "sub_name is empty")
	}
	grouped, _ := strconv.ParseBool(c.QueryParam("grouped")) // defaults to false if parameter is missing or invalid

	return s.handleClashProxyProvider(c, subName, grouped)
}

func (s *Server) handleClashProxyProvider(c echo.Context, subName string, grouped bool) error {
	if s.Reloader != nil {
		if err := s.Reloader.Reload(true); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
	} else {
		s.l.Debugf("Reloader is nil this should not happen")
		return echo.NewHTTPError(http.StatusBadRequest, "should not happen error happen :)")
	}

	clashSubList, err := s.cfg.GetClashSubList()
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	for _, clashSub := range clashSubList {
		if clashSub.Name == subName {
			var clashCfgBuf []byte
			if grouped {
				clashCfgBuf, err = clashSub.ToGroupedClashConfigYaml()
			} else {
				clashCfgBuf, err = clashSub.ToClashConfigYaml()
			}
			if err != nil {
				return c.JSON(http.StatusBadRequest, map[string]string{"message": err.Error()})
			}
			return c.String(http.StatusOK, string(clashCfgBuf))
		}
	}
	msg := fmt.Sprintf("sub_name=%s not found", subName)
	return c.JSON(http.StatusBadRequest, map[string]string{"message": msg})
}

func (s *Server) HandleReload(c echo.Context) error {
	if s.Reloader == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "reload not support")
	}
	err := s.Reloader.Reload(true)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if _, err := c.Response().Write([]byte("reload success")); err != nil {
		s.l.Errorf("write response meet err=%v", err)
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return nil
}

func (s *Server) HandleHealthCheck(c echo.Context) error {
	relayLabel := c.QueryParam("relay_label")
	if relayLabel == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "relay_label is required")
	}
	if err := s.HealthCheck(c.Request().Context(), relayLabel); err != nil {
		res := CommonResp{Message: err.Error()}
		return c.JSON(http.StatusBadRequest, res)
	}
	return c.JSON(http.StatusOK, CommonResp{Message: "connect success"})
}

func (s *Server) CurrentConfig(c echo.Context) error {
	ret, err := json.Marshal(s.cfg)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSONBlob(http.StatusOK, ret)
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
