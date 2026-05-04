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

// JobStatus is the in-memory record of the most-recent update attempt.
// Process-local on purpose: after a successful restart the new process
// boots with no record, the SPA reloads /version and sees the new build.
type JobStatus struct {
	State     updater.State `json:"state"`
	Channel   string        `json:"channel"`
	From      string        `json:"from"`
	To        string        `json:"to"`
	StartedAt time.Time     `json:"started_at"`
	Error     string        `json:"error,omitempty"`
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
	res, err := updater.Check(ctx, channel, constant.Version)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	return c.JSON(http.StatusOK, res)
}

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

	prev := s.updateJob.Load()
	if prev != nil && isInProgress(prev.State) {
		return echo.NewHTTPError(http.StatusConflict, "another update is already running")
	}

	job := &JobStatus{
		State:     updater.StateChecking,
		Channel:   opts.Channel,
		From:      constant.Version,
		StartedAt: time.Now().UTC(),
	}
	s.updateJob.Store(job)
	s.l.Infof("update apply requested channel=%s force=%v restart=%v", opts.Channel, opts.Force, opts.Restart)

	// Detached context: closing the browser shouldn't abort an in-flight swap.
	go s.runUpdate(opts, job)
	return c.JSON(http.StatusAccepted, map[string]string{"state": string(updater.StateChecking)})
}

func (s *Server) runUpdate(opts updater.ApplyOptions, job *JobStatus) {
	ctx, cancel := context.WithTimeout(context.Background(), updateApplyTimeout)
	defer cancel()

	onState := func(st updater.State) {
		// Copy-on-write so /status readers always see a consistent snapshot.
		next := *job
		next.State = st
		s.updateJob.Store(&next)
		*job = next
	}

	if err := updater.Apply(ctx, opts, constant.Version, s.l, onState); err != nil {
		next := *job
		next.State = updater.StateFailed
		next.Error = err.Error()
		s.updateJob.Store(&next)
		s.l.Errorf("update failed: %v", err)
	}
}

func (s *Server) UpdateStatus(c echo.Context) error {
	if j := s.updateJob.Load(); j != nil {
		return c.JSON(http.StatusOK, j)
	}
	return c.JSON(http.StatusOK, map[string]string{"state": "idle"})
}

func isInProgress(s updater.State) bool {
	switch s {
	case updater.StateChecking, updater.StateDownloading, updater.StateInstalling, updater.StateRestarting:
		return true
	}
	return false
}
