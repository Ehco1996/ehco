package web

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Ehco1996/ehco/internal/cmgr/ms"
	"github.com/labstack/echo/v4"
)

const (
	defaultTimeRange = 60 // seconds
	errInvalidParam  = "invalid parameter: %s"
)

type queryParams struct {
	startTS int64
	endTS   int64
	refresh bool
}

func parseQueryParams(c echo.Context) (*queryParams, error) {
	now := time.Now().Unix()
	params := &queryParams{
		startTS: now - defaultTimeRange,
		endTS:   now,
		refresh: false,
	}

	if start, err := parseTimestamp(c.QueryParam("start_ts")); err == nil {
		params.startTS = start
	}

	if end, err := parseTimestamp(c.QueryParam("end_ts")); err == nil {
		params.endTS = end
	}

	if refresh, err := strconv.ParseBool(c.QueryParam("latest")); err == nil {
		params.refresh = refresh
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
	req := &ms.QueryNodeMetricsReq{StartTimestamp: params.startTS, EndTimestamp: params.endTS, Num: -1}
	if params.refresh {
		req.Num = 1
	}
	metrics, err := s.connMgr.QueryNodeMetrics(c.Request().Context(), req, params.refresh)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, metrics)
}

func (s *Server) GetRuleMetrics(c echo.Context) error {
	params, err := parseQueryParams(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	req := &ms.QueryRuleMetricsReq{
		StartTimestamp: params.startTS,
		EndTimestamp:   params.endTS,
		Num:            -1,
		RuleLabel:      c.QueryParam("label"),
		Remote:         c.QueryParam("remote"),
	}
	if params.refresh {
		req.Num = 1
	}

	metrics, err := s.connMgr.QueryRuleMetrics(c.Request().Context(), req, params.refresh)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, metrics)
}
