// Package updater self-updates the ehco binary from GitHub releases.
// Used by both `ehco update` (CLI) and the dashboard's /api/v1/update/*
// endpoints.
package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/mod/semver"
)

const (
	ChannelAuto    = "auto"
	ChannelStable  = "stable"
	ChannelNightly = "nightly"

	releasesAPI        = "https://api.github.com/repos/Ehco1996/ehco/releases"
	systemdServiceName = "ehco"
)

// State is the phase of an Apply run; consumed by the web UI.
type State string

const (
	StateChecking    State = "checking"
	StateDownloading State = "downloading"
	StateInstalling  State = "installing"
	StateRestarting  State = "restarting"
	StateDone        State = "done"
	StateFailed      State = "failed"
)

// CheckResult describes a release relative to the running binary.
type CheckResult struct {
	Channel         string    `json:"channel"`
	CurrentVersion  string    `json:"current_version"`
	LatestVersion   string    `json:"latest_version"`
	LatestTag       string    `json:"latest_tag"`
	ReleaseName     string    `json:"release_name"`
	ReleaseBody     string    `json:"release_body"`
	ReleaseURL      string    `json:"release_url"`
	PublishedAt     time.Time `json:"published_at"`
	UpdateAvailable bool      `json:"update_available"`
	AssetName       string    `json:"asset_name"`
	AssetURL        string    `json:"asset_url"`
}

// ApplyOptions doubles as the JSON body of POST /api/v1/update/apply.
type ApplyOptions struct {
	Channel string `json:"channel"`
	Force   bool   `json:"force"`
	Restart bool   `json:"restart"`
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type ghRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	Prerelease  bool      `json:"prerelease"`
	Draft       bool      `json:"draft"`
	PublishedAt time.Time `json:"published_at"`
	HTMLURL     string    `json:"html_url"`
	Assets      []ghAsset `json:"assets"`
}

// Check resolves channel against currentVersion and queries GitHub.
// currentRevision is the ldflag-injected constant.GitRevision; empty is
// tolerated (we'll just trust version-string equality in that case).
func Check(ctx context.Context, channel, currentVersion, currentRevision string) (*CheckResult, error) {
	resolved, rel, err := pickRelease(ctx, channel, currentVersion)
	if err != nil {
		return nil, err
	}
	latest := strings.TrimPrefix(rel.TagName, "v")
	res := &CheckResult{
		Channel:        resolved,
		CurrentVersion: currentVersion,
		LatestVersion:  latest,
		LatestTag:      rel.TagName,
		ReleaseName:    rel.Name,
		ReleaseBody:    rel.Body,
		ReleaseURL:     rel.HTMLURL,
		PublishedAt:    rel.PublishedAt,
	}
	if latest == currentVersion {
		res.UpdateAvailable = nightlyRepublished(ctx, rel, currentRevision)
	} else {
		res.UpdateAvailable = compareVersions(latest, currentVersion) > 0
	}
	if a := pickAsset(rel.Assets); a != nil {
		res.AssetName = a.Name
		res.AssetURL = a.BrowserDownloadURL
	}
	return res, nil
}

// Apply downloads + swaps + (optionally) restarts. Each phase is reported
// to onState so the dashboard can render progress; CLI passes nil.
func Apply(ctx context.Context, opts ApplyOptions, currentVersion, currentRevision string, log *zap.SugaredLogger, onState func(State)) error {
	emit := func(s State) {
		if onState != nil {
			onState(s)
		}
	}

	emit(StateChecking)
	resolved, rel, err := pickRelease(ctx, opts.Channel, currentVersion)
	if err != nil {
		return err
	}
	latest := strings.TrimPrefix(rel.TagName, "v")
	log.Infof("channel=%s current=%s latest=%s", resolved, currentVersion, latest)

	if !opts.Force {
		if latest == currentVersion {
			if nightlyRepublished(ctx, rel, currentRevision) {
				log.Infof("nightly tag %s now points at a different commit than local %s; reinstalling",
					rel.TagName, currentRevision)
			} else {
				log.Info("already up to date")
				emit(StateDone)
				return nil
			}
		} else if compareVersions(latest, currentVersion) < 0 {
			return fmt.Errorf("refusing to downgrade %s -> %s; use force", currentVersion, latest)
		}
	}

	asset := pickAsset(rel.Assets)
	if asset == nil {
		return fmt.Errorf("no release asset for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	binPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate binary: %w", err)
	}
	if binPath, err = filepath.EvalSymlinks(binPath); err != nil {
		return fmt.Errorf("resolve binary symlink: %w", err)
	}
	tmpPath := binPath + ".new"

	emit(StateDownloading)
	log.Infof("downloading %s -> %s", asset.BrowserDownloadURL, tmpPath)
	if err := download(ctx, asset.BrowserDownloadURL, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("download: %w", err)
	}

	emit(StateInstalling)
	// rename(2) over a running ELF on linux is safe: the kernel keeps the
	// old inode alive for the running process while new invocations
	// resolve to the new file.
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("chmod: %w", err)
	}
	if err := os.Rename(tmpPath, binPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace %s: %w", binPath, err)
	}
	log.Infof("installed %s at %s", latest, binPath)

	if !opts.Restart {
		log.Info("skipping restart; restart manually to pick up the new binary")
		emit(StateDone)
		return nil
	}
	emit(StateRestarting)
	if err := restartSystemd(log); err != nil {
		return err
	}
	emit(StateDone)
	return nil
}

func pickRelease(ctx context.Context, channel, currentVersion string) (string, *ghRelease, error) {
	resolved, err := resolveChannel(channel, currentVersion)
	if err != nil {
		return "", nil, err
	}
	rel, err := fetchLatest(ctx, resolved)
	if err != nil {
		return "", nil, fmt.Errorf("fetch %s: %w", resolved, err)
	}
	return resolved, rel, nil
}

func resolveChannel(flag, currentVersion string) (string, error) {
	switch flag {
	case ChannelStable, ChannelNightly:
		return flag, nil
	case ChannelAuto, "":
		// goreleaser injects "1.1.7-next" for nightlies, "1.1.7" for
		// stable. semver.Prerelease handles "+build" and "-rc.1" too.
		if semver.Prerelease("v"+currentVersion) != "" {
			return ChannelNightly, nil
		}
		return ChannelStable, nil
	default:
		return "", fmt.Errorf("invalid channel %q (auto|stable|nightly)", flag)
	}
}

func fetchLatest(ctx context.Context, channel string) (*ghRelease, error) {
	if channel == ChannelStable {
		// /releases/latest excludes prereleases, perfect for stable.
		var rel ghRelease
		if err := getJSON(ctx, releasesAPI+"/latest", &rel); err != nil {
			return nil, err
		}
		if rel.TagName == "" {
			return nil, fmt.Errorf("empty tag in github response")
		}
		return &rel, nil
	}
	// Nightly: list releases and pick the freshest prerelease.
	var all []ghRelease
	if err := getJSON(ctx, releasesAPI+"?per_page=30", &all); err != nil {
		return nil, err
	}
	var best *ghRelease
	for i := range all {
		r := &all[i]
		if r.Draft || !r.Prerelease {
			continue
		}
		if best == nil || r.PublishedAt.After(best.PublishedAt) {
			best = r
		}
	}
	if best == nil {
		return nil, fmt.Errorf("no nightly release found")
	}
	return best, nil
}

func getJSON(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("github %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// nightlyRepublished reports whether a release whose tag matches the
// running version actually points at a different commit than the
// running binary. Nightly uses a rolling tag (v1.1.7-next), so
// version-string equality alone would mask republished builds.
//
// We can't time-compare published_at vs BuildTime: goreleaser publishes
// the release a few seconds after building the artifact, so the same
// artifact would always look "older" than its own release. SHA is the
// only reliable signal.
//
// Conservative on uncertainty: empty currentRevision (bare `go build`)
// or GitHub fetch failure -> false (no spurious update prompts; user
// can --force).
func nightlyRepublished(ctx context.Context, rel *ghRelease, currentRevision string) bool {
	if !rel.Prerelease || currentRevision == "" {
		return false
	}
	sha, err := fetchTagCommitSHA(ctx, rel.TagName)
	if err != nil || sha == "" {
		return false
	}
	return !shaMatchesRevision(sha, currentRevision)
}

// shaMatchesRevision compares the short revision baked into the binary
// (goreleaser uses {{.ShortCommit}}, 7 chars; Makefile uses full SHA)
// against the full SHA returned by GitHub. Prefix-match is enough.
func shaMatchesRevision(fullSHA, currentRevision string) bool {
	n := len(currentRevision)
	if n == 0 || n > len(fullSHA) {
		return false
	}
	return strings.EqualFold(fullSHA[:n], currentRevision)
}

func fetchTagCommitSHA(ctx context.Context, tag string) (string, error) {
	var c struct {
		SHA string `json:"sha"`
	}
	url := "https://api.github.com/repos/Ehco1996/ehco/commits/" + tag
	if err := getJSON(ctx, url, &c); err != nil {
		return "", err
	}
	return c.SHA, nil
}

// compareVersions returns -1/0/1 like semver.Compare. Falls back to
// string compare for unparseable versions so a malformed constant.Version
// never crashes the updater (--force still works).
func compareVersions(a, b string) int {
	va, vb := "v"+strings.TrimPrefix(a, "v"), "v"+strings.TrimPrefix(b, "v")
	if semver.IsValid(va) && semver.IsValid(vb) {
		return semver.Compare(va, vb)
	}
	return strings.Compare(a, b)
}

func pickAsset(assets []ghAsset) *ghAsset {
	if runtime.GOOS != "linux" {
		return nil
	}
	want := fmt.Sprintf("ehco_linux_%s", runtime.GOARCH)
	for i := range assets {
		if assets[i].Name == want {
			return &assets[i]
		}
	}
	return nil
}

func download(ctx context.Context, url, dst string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s", resp.Status)
	}
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func restartSystemd(log *zap.SugaredLogger) error {
	if _, err := exec.LookPath("systemctl"); err != nil {
		log.Warn("systemctl not found; restart ehco manually")
		return nil
	}
	log.Infof("restarting %s.service via systemctl", systemdServiceName)
	cmd := exec.Command("systemctl", "restart", systemdServiceName+".service")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
