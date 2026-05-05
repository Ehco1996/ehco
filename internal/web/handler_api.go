package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Ehco1996/ehco/internal/cmgr"
	"github.com/Ehco1996/ehco/internal/cmgr/ms"
	"github.com/Ehco1996/ehco/internal/glue"
	"github.com/labstack/echo/v4"
)

const (
	defaultTimeRange = 60 // seconds
	errInvalidParam  = "invalid parameter: %s"
)

type queryParams struct {
	startTS int64
	endTS   int64
	latest  bool
	step    int64
}

func parseQueryParams(c echo.Context) (*queryParams, error) {
	now := time.Now().Unix()
	params := &queryParams{
		startTS: now - defaultTimeRange,
		endTS:   now,
	}

	if start, err := parseTimestamp(c.QueryParam("start_ts")); err == nil {
		params.startTS = start
	}

	if end, err := parseTimestamp(c.QueryParam("end_ts")); err == nil {
		params.endTS = end
	}

	if latest, err := strconv.ParseBool(c.QueryParam("latest")); err == nil {
		params.latest = latest
	}

	if step, err := strconv.ParseInt(c.QueryParam("step"), 10, 64); err == nil && step > 0 {
		params.step = step
	}

	if params.startTS >= params.endTS {
		return nil, fmt.Errorf(errInvalidParam, "time range")
	}

	return params, nil
}

func parseTimestamp(s string) (int64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty timestamp")
	}
	return strconv.ParseInt(s, 10, 64)
}

func (s *Server) GetNodeMetrics(c echo.Context) error {
	params, err := parseQueryParams(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	req := &ms.QueryNodeMetricsReq{StartTimestamp: params.startTS, EndTimestamp: params.endTS, Num: -1, Step: params.step}
	if params.latest {
		req.Num = 1
	}
	metrics, err := s.connMgr.QueryNodeMetrics(c.Request().Context(), req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, metrics)
}

// AuthInfo reports whether the server requires login and whether the
// current request already carries a valid session/bearer. The SPA boots
// off this — if auth_required is false it skips LoginGate entirely; if
// authenticated is true on first hit (e.g. cookie still valid) it goes
// straight to the dashboard.
//
// Public — must remain reachable without auth so the SPA can probe.
func (s *Server) AuthInfo(c echo.Context) error {
	authenticated, _, _ := s.auth.checkRequest(c.Request())
	return c.JSON(http.StatusOK, map[string]bool{
		"auth_required": s.auth.authRequired(),
		"authenticated": authenticated,
	})
}

func (s *Server) CurrentConfig(c echo.Context) error {
	ret, err := json.Marshal(s.cfg)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSONBlob(http.StatusOK, ret)
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

// OverviewResp bundles everything the SPA's home page polls — saves
// the front-end the 3 parallel fetches it would otherwise need on
// every refresh tick. Fields stay nil/zero when their subsystem is
// disabled (xray-less deployments, no host sampler yet).
type OverviewResp struct {
	Xray  *glue.XraySnapshot `json:"xray,omitempty"`
	Host  *ms.NodeMetrics    `json:"host,omitempty"`
	Rules int                `json:"rules"`
}

func (s *Server) Overview(c echo.Context) error {
	out := OverviewResp{}

	if s.cfg != nil {
		out.Rules = len(s.cfg.RelayConfigs)
	}

	if p := s.xrayStatus.Load(); p != nil && *p != nil {
		snap := (*p).Snapshot()
		out.Xray = &snap
	}

	if s.connMgr != nil {
		now := time.Now()
		req := &ms.QueryNodeMetricsReq{
			StartTimestamp: now.Add(-5 * time.Minute).Unix(),
			EndTimestamp:   now.Unix(),
			Num:            1,
		}
		if resp, err := s.connMgr.QueryNodeMetrics(c.Request().Context(), req); err == nil && len(resp.Data) > 0 {
			h := resp.Data[0]
			out.Host = &h
		}
	}

	return c.JSON(http.StatusOK, out)
}

// dbMaintenanceErr maps domain errors from the cmgr/ms layer onto echo
// HTTP errors. Centralised so every db/* handler treats the same error
// the same way.
func dbMaintenanceErr(err error) *echo.HTTPError {
	switch {
	case errors.Is(err, cmgr.ErrMetricsDisabled):
		return echo.NewHTTPError(http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, ms.ErrTruncateNotConfirmed):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	default:
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
}

func (s *Server) GetDBHealth(c echo.Context) error {
	h, err := s.connMgr.DBHealth(c.Request().Context())
	if err != nil {
		return dbMaintenanceErr(err)
	}
	return c.JSON(http.StatusOK, h)
}

type dbCleanupReq struct {
	OlderThanDays int `json:"older_than_days"`
}

func (s *Server) PostDBCleanup(c echo.Context) error {
	var req dbCleanupReq
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	res, err := s.connMgr.DBCleanup(c.Request().Context(), req.OlderThanDays)
	if err != nil {
		return dbMaintenanceErr(err)
	}
	return c.JSON(http.StatusOK, res)
}

func (s *Server) PostDBVacuum(c echo.Context) error {
	res, err := s.connMgr.DBVacuum(c.Request().Context())
	if err != nil {
		return dbMaintenanceErr(err)
	}
	return c.JSON(http.StatusOK, res)
}

type dbTruncateReq struct {
	Confirm string `json:"confirm"`
}

func (s *Server) PostDBTruncate(c echo.Context) error {
	var req dbTruncateReq
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	res, err := s.connMgr.DBTruncate(c.Request().Context(), req.Confirm)
	if err != nil {
		return dbMaintenanceErr(err)
	}
	return c.JSON(http.StatusOK, res)
}

func (s *Server) PostDBResetStats(c echo.Context) error {
	if err := s.connMgr.DBResetStats(); err != nil {
		return dbMaintenanceErr(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (s *Server) HandleHealthCheck(c echo.Context) error {
	relayLabel := c.QueryParam("relay_label")
	if relayLabel == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "relay_label is required")
	}
	latency, err := s.HealthCheck(c.Request().Context(), relayLabel)
	if err != nil {
		res := HealthCheckResp{Message: err.Error(), ErrorCode: -1}
		return c.JSON(http.StatusBadRequest, res)
	}
	return c.JSON(http.StatusOK, HealthCheckResp{Message: "connect success", Latency: latency})
}
