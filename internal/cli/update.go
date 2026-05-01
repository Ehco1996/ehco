package cli

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

	"github.com/Ehco1996/ehco/internal/constant"
	cli "github.com/urfave/cli/v2"
	"golang.org/x/mod/semver"
)

const (
	githubLatestReleaseAPI = "https://api.github.com/repos/Ehco1996/ehco/releases/latest"
	githubReleasesAPI      = "https://api.github.com/repos/Ehco1996/ehco/releases?per_page=30"
	systemdServiceName     = "ehco"

	channelAuto    = "auto"
	channelStable  = "stable"
	channelNightly = "nightly"

	nightlyTagSuffix = "-next"
)

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type ghRelease struct {
	TagName     string    `json:"tag_name"`
	Prerelease  bool      `json:"prerelease"`
	Draft       bool      `json:"draft"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []ghAsset `json:"assets"`
}

var UpdateCMD = &cli.Command{
	Name:  "update",
	Usage: "update ehco to the latest GitHub release and restart the systemd service",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "force",
			Usage: "force update even if already at the latest version, or to allow downgrade / channel switch",
		},
		&cli.BoolFlag{
			Name:  "no-restart",
			Usage: "skip systemctl restart after replacing the binary",
		},
		&cli.StringFlag{
			Name:  "channel",
			Value: channelAuto,
			Usage: "release channel to track: auto (match current build), stable, or nightly",
		},
	},
	Action: runUpdate,
}

func runUpdate(c *cli.Context) error {
	ctx, cancel := context.WithTimeout(c.Context, 5*time.Minute)
	defer cancel()

	channel, err := resolveChannel(c.String("channel"), constant.Version)
	if err != nil {
		return err
	}

	rel, err := fetchTargetRelease(ctx, channel)
	if err != nil {
		return fmt.Errorf("fetch %s release: %w", channel, err)
	}
	latest := strings.TrimPrefix(rel.TagName, "v")
	cliLogger.Infof("channel=%s current version=%s latest version=%s", channel, constant.Version, latest)

	force := c.Bool("force")
	if !force {
		if latest == constant.Version {
			cliLogger.Info("already up to date, nothing to do")
			return nil
		}
		if cmp := compareVersions(latest, constant.Version); cmp < 0 {
			return fmt.Errorf("refusing to downgrade from %s to %s; rerun with --force to override",
				constant.Version, latest)
		}
	}

	asset, err := pickReleaseAsset(rel.Assets)
	if err != nil {
		return err
	}

	binPath, err := resolveBinaryPath()
	if err != nil {
		return fmt.Errorf("resolve current binary path: %w", err)
	}
	tmpPath := binPath + ".new"
	cliLogger.Infof("downloading %s -> %s", asset.BrowserDownloadURL, tmpPath)
	if err := downloadFile(ctx, asset.BrowserDownloadURL, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("download asset: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("chmod new binary: %w", err)
	}
	// rename(2) over a running ELF on linux is safe: the kernel keeps the
	// old inode alive for the existing process, while new invocations
	// (including the post-restart service) resolve to the new file.
	if err := os.Rename(tmpPath, binPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace binary at %s: %w", binPath, err)
	}
	cliLogger.Infof("binary at %s updated to version %s", binPath, latest)

	if c.Bool("no-restart") {
		cliLogger.Info("skipping systemd restart (--no-restart); restart ehco manually to pick up the new binary")
		return nil
	}
	return restartSystemdService()
}

func resolveChannel(flagVal, currentVersion string) (string, error) {
	switch flagVal {
	case channelStable, channelNightly:
		return flagVal, nil
	case channelAuto, "":
		if isNightlyVersion(currentVersion) {
			return channelNightly, nil
		}
		return channelStable, nil
	default:
		return "", fmt.Errorf("invalid --channel %q (want one of auto, stable, nightly)", flagVal)
	}
}

func isNightlyVersion(v string) bool {
	// goreleaser injects bare versions like "1.1.7-next" or "1.1.6"; both
	// stable and nightly builds skip the leading "v". A nightly is anything
	// carrying a prerelease suffix (currently "-next"), but we use a generic
	// "contains a dash" check so future suffixes (e.g. "-rc.1") still work.
	return strings.Contains(v, "-")
}

func fetchTargetRelease(ctx context.Context, channel string) (*ghRelease, error) {
	switch channel {
	case channelStable:
		return fetchLatestStableRelease(ctx)
	case channelNightly:
		return fetchLatestNightlyRelease(ctx)
	default:
		return nil, fmt.Errorf("unknown channel %q", channel)
	}
}

func fetchLatestStableRelease(ctx context.Context) (*ghRelease, error) {
	var rel ghRelease
	if err := getJSON(ctx, githubLatestReleaseAPI, &rel); err != nil {
		return nil, err
	}
	if rel.TagName == "" {
		return nil, fmt.Errorf("empty tag name in github response")
	}
	return &rel, nil
}

func fetchLatestNightlyRelease(ctx context.Context) (*ghRelease, error) {
	// /releases/latest excludes prereleases by design, so list recent
	// releases and pick the freshest nightly ourselves.
	var all []ghRelease
	if err := getJSON(ctx, githubReleasesAPI, &all); err != nil {
		return nil, err
	}
	var best *ghRelease
	for i := range all {
		r := &all[i]
		if r.Draft || !r.Prerelease {
			continue
		}
		if !strings.HasSuffix(r.TagName, nightlyTagSuffix) {
			continue
		}
		if best == nil || r.PublishedAt.After(best.PublishedAt) {
			best = r
		}
	}
	if best == nil {
		return nil, fmt.Errorf("no nightly release found (looking for tags ending in %q)", nightlyTagSuffix)
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
		return fmt.Errorf("github api %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// compareVersions returns -1/0/1 like semver.Compare. Inputs may have or
// omit the leading "v"; unparseable inputs fall back to string compare so a
// malformed version never crashes the updater (it just disables the
// downgrade guard for that case, which --force handles).
func compareVersions(a, b string) int {
	va, vb := ensureV(a), ensureV(b)
	if semver.IsValid(va) && semver.IsValid(vb) {
		return semver.Compare(va, vb)
	}
	return strings.Compare(a, b)
}

func ensureV(s string) string {
	if strings.HasPrefix(s, "v") {
		return s
	}
	return "v" + s
}

func pickReleaseAsset(assets []ghAsset) (*ghAsset, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("update only supports linux releases, current os=%s", runtime.GOOS)
	}
	want := fmt.Sprintf("ehco_linux_%s", runtime.GOARCH)
	for i := range assets {
		if assets[i].Name == want {
			return &assets[i], nil
		}
	}
	return nil, fmt.Errorf("no release asset matches %s", want)
}

func resolveBinaryPath() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", err
	}
	return resolved, nil
}

func downloadFile(ctx context.Context, url, dst string) error {
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
		return fmt.Errorf("download returned %s", resp.Status)
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

func restartSystemdService() error {
	if _, err := exec.LookPath("systemctl"); err != nil {
		cliLogger.Warn("systemctl not found on PATH; please restart ehco manually")
		return nil
	}
	unit := systemdServiceName + ".service"
	cliLogger.Infof("restarting %s via systemctl", unit)
	cmd := exec.Command("systemctl", "restart", unit)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
