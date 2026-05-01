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
)

const (
	githubLatestReleaseAPI = "https://api.github.com/repos/Ehco1996/ehco/releases/latest"
	systemdServiceName     = "ehco"
)

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

var UpdateCMD = &cli.Command{
	Name:  "update",
	Usage: "update ehco to the latest GitHub release and restart the systemd service",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "force",
			Usage: "force update even if already at the latest version",
		},
		&cli.BoolFlag{
			Name:  "no-restart",
			Usage: "skip systemctl restart after replacing the binary",
		},
	},
	Action: runUpdate,
}

func runUpdate(c *cli.Context) error {
	ctx, cancel := context.WithTimeout(c.Context, 5*time.Minute)
	defer cancel()

	rel, err := fetchLatestRelease(ctx)
	if err != nil {
		return fmt.Errorf("fetch latest release: %w", err)
	}
	latest := strings.TrimPrefix(rel.TagName, "v")
	cliLogger.Infof("current version=%s latest version=%s", constant.Version, latest)
	if !c.Bool("force") && latest == constant.Version {
		cliLogger.Info("already up to date, nothing to do")
		return nil
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

func fetchLatestRelease(ctx context.Context) (*ghRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubLatestReleaseAPI, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("github api %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	if rel.TagName == "" {
		return nil, fmt.Errorf("empty tag name in github response")
	}
	return &rel, nil
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
