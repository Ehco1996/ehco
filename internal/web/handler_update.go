package web

import (
	"context"
	"net/http"
	"runtime"
	"time"

	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/updater"
	"github.com/labstack/echo/v4"
)

const updateApplyTimeout = 5 * time.Minute

type VersionInfo struct {
	Version     string    `json:"version"`
	GitBranch   string    `json:"git_branch"`
	GitRevision string    `json:"git_revision"`
	BuildTime   string    `json:"build_time"`
	StartTime   time.Time `json:"start_time"`
	GoOS        string    `json:"go_os"`
	GoArch      string    `json:"go_arch"`
}

func (s *Server) Version(c echo.Context) error {
	return c.JSON(http.StatusOK, VersionInfo{
		Version:     constant.Version,
		GitBranch:   constant.GitBranch,
		GitRevision: constant.GitRevision,
		BuildTime:   constant.BuildTime,
		StartTime:   constant.StartTime,
		GoOS:        runtime.GOOS,
		GoArch:      runtime.GOARCH,
	})
}

func (s *Server) UpdateCheck(c echo.Context) error {
	channel := c.QueryParam("channel")
	if channel == "" {
		channel = updater.ChannelAuto
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), 30*time.Second)
	defer cancel()
	res, err := updater.Check(ctx, channel, constant.Version, constant.GitRevision)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	return c.JSON(http.StatusOK, res)
}

// UpdateApply kicks off the update in a detached goroutine and returns
// immediately. The dashboard polls /version to detect completion (the
// running process restarts mid-flow, so any in-process state machine is
// inherently lossy). Failures are logged via s.l; check journalctl.
func (s *Server) UpdateApply(c echo.Context) error {
	if runtime.GOOS != "linux" {
		return echo.NewHTTPError(http.StatusBadRequest,
			"self-update only supported on linux; current platform is "+runtime.GOOS)
	}
	var opts updater.ApplyOptions
	if err := c.Bind(&opts); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if opts.Channel == "" {
		opts.Channel = updater.ChannelAuto
	}
	s.l.Infof("update apply requested channel=%s force=%v restart=%v", opts.Channel, opts.Force, opts.Restart)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), updateApplyTimeout)
		defer cancel()
		if err := updater.Apply(ctx, opts, constant.Version, constant.GitRevision, s.l); err != nil {
			s.l.Errorf("update failed: %v", err)
		}
	}()
	return c.NoContent(http.StatusAccepted)
}
