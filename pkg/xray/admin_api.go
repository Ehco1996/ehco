package xray

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
)

// RegisterRoutes mounts the xray management endpoints onto the given echo group.
// Authentication is provided by the surrounding web server's middleware.
//
//	GET    /xray/conns           — list active conns (optional ?user=<id>)
//	DELETE /xray/conns/:id       — kill a single conn by id
//	DELETE /xray/conns           — kill all conns for ?user=<id>
func (xs *XrayServer) RegisterRoutes(g *echo.Group) {
	g.GET("/xray/conns", xs.listConns)
	g.DELETE("/xray/conns/:id", xs.killConn)
	g.DELETE("/xray/conns", xs.killUserConns)
}

func (xs *XrayServer) listConns(c echo.Context) error {
	userID := 0
	if v := c.QueryParam("user"); v != "" {
		id, err := strconv.Atoi(v)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
		}
		userID = id
	}
	return c.JSON(http.StatusOK, xs.tracker.List(userID))
}

func (xs *XrayServer) killConn(c echo.Context) error {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid conn id")
	}
	if !xs.tracker.Kill(id) {
		return echo.NewHTTPError(http.StatusNotFound, "conn not found")
	}
	return c.JSON(http.StatusOK, map[string]any{"killed": 1, "id": id})
}

func (xs *XrayServer) killUserConns(c echo.Context) error {
	v := c.QueryParam("user")
	if v == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing user query param")
	}
	id, err := strconv.Atoi(v)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
	}
	n := xs.tracker.KillByUser(id)
	return c.JSON(http.StatusOK, map[string]any{"killed": n, "user_id": id})
}
