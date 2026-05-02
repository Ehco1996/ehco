package xray

import (
	"net/http"
	"sort"
	"strconv"
	"sync/atomic"

	"github.com/labstack/echo/v4"
)

// RegisterRoutes mounts the xray management endpoints onto the given echo group.
// Authentication is provided by the surrounding web server's middleware.
//
//	GET    /xray/conns           — list active conns (optional ?user=<id>)
//	DELETE /xray/conns/:id       — kill a single conn by id
//	DELETE /xray/conns           — kill all conns for ?user=<id>
//	GET    /xray/users           — read-only snapshot of every known user with
//	                               cumulative-since-boot byte counters and
//	                               live tcp conn count + recent IPs
func (xs *XrayServer) RegisterRoutes(g *echo.Group) {
	g.GET("/xray/conns", xs.listConns)
	g.DELETE("/xray/conns/:id", xs.killConn)
	g.DELETE("/xray/conns", xs.killUserConns)
	g.GET("/xray/users", xs.listUsers)
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

// UserView is the read-only shape returned by GET /xray/users. Distinct from
// the cycle-reset UserTraffic struct used by syncTrafficToServer.
type UserView struct {
	ID            int      `json:"user_id"`
	Level         int      `json:"level"`
	Protocol      string   `json:"protocol"`
	Method        string   `json:"method"`
	Enable        bool     `json:"enable"`
	Running       bool     `json:"running"`
	UploadTotal   int64    `json:"upload_total"`
	DownloadTotal int64    `json:"download_total"`
	TcpConnCount  int      `json:"tcp_conn_count"`
	RecentIPs     []string `json:"recent_ips"`
}

func (xs *XrayServer) listUsers(c echo.Context) error {
	if xs.up == nil {
		return c.JSON(http.StatusOK, []UserView{})
	}

	users := xs.up.GetAllUsers()
	out := make([]UserView, 0, len(users))
	for _, u := range users {
		view := UserView{
			ID:            u.ID,
			Level:         u.Level,
			Protocol:      u.Protocol,
			Method:        u.Method,
			Enable:        u.Enable,
			Running:       u.running,
			UploadTotal:   atomic.LoadInt64(&u.UploadTotal),
			DownloadTotal: atomic.LoadInt64(&u.DownloadTotal),
		}
		if xs.tracker != nil {
			view.TcpConnCount = xs.tracker.CountTCPByUser(u.ID)
			ips := make([]string, 0)
			seen := make(map[string]struct{})
			for _, ci := range xs.tracker.List(u.ID) {
				if ci.SourceIP == "" {
					continue
				}
				if _, ok := seen[ci.SourceIP]; ok {
					continue
				}
				seen[ci.SourceIP] = struct{}{}
				ips = append(ips, ci.SourceIP)
			}
			view.RecentIPs = ips
		}
		if view.RecentIPs == nil {
			view.RecentIPs = []string{}
		}
		out = append(out, view)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return c.JSON(http.StatusOK, out)
}
