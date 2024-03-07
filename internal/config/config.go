package config

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Ehco1996/ehco/internal/relay/conf"
	"github.com/Ehco1996/ehco/pkg/sub"
	xConf "github.com/xtls/xray-core/infra/conf"
	"go.uber.org/zap"
)

type Config struct {
	PATH string

	WebHost     string `json:"web_host,omitempty"`
	WebPort     int    `json:"web_port,omitempty"`
	WebToken    string `json:"web_token,omitempty"`
	WebAuthUser string `json:"web_auth_user,omitempty"`
	WebAuthPass string `json:"web_auth_pass,omitempty"`

	LogLeveL       string `json:"log_level,omitempty"`
	EnablePing     bool   `json:"enable_ping,omitempty"`
	ReloadInterval int    `json:"reload_interval,omitempty"`

	RelayConfigs      []*conf.Config `json:"relay_configs"`
	RelaySyncURL      string         `json:"relay_sync_url,omitempty"`
	RelaySyncInterval int            `json:"relay_sync_interval,omitempty"`

	SubConfigs          []*SubConfig  `json:"sub_configs,omitempty"`
	XRayConfig          *xConf.Config `json:"xray_config,omitempty"`
	SyncTrafficEndPoint string        `json:"sync_traffic_endpoint,omitempty"`

	lastLoadTime time.Time
	l            *zap.SugaredLogger

	cachedClashSubMap map[string]*sub.ClashSub // key: clash sub name
}

func NewConfig(path string) *Config {
	return &Config{PATH: path, l: zap.S().Named("cfg"), cachedClashSubMap: make(map[string]*sub.ClashSub)}
}

func (c *Config) NeedSyncFromServer() bool {
	return strings.Contains(c.PATH, "http")
}

func (c *Config) LoadConfig() error {
	if c.ReloadInterval > 0 && time.Since(c.lastLoadTime).Seconds() < float64(c.ReloadInterval) {
		c.l.Warnf("Skip Load Config, last load time: %s", c.lastLoadTime)
		return nil
	}
	// reset
	c.RelayConfigs = nil
	c.lastLoadTime = time.Now()
	if c.NeedSyncFromServer() {
		if err := c.readFromHttp(); err != nil {
			return err
		}
	} else {
		if err := c.readFromFile(); err != nil {
			return err
		}
	}
	return c.Adjust()
}

func (c *Config) readFromFile() error {
	file, err := os.ReadFile(c.PATH)
	if err != nil {
		return err
	}
	c.l.Infof("Load Config From File: %s", c.PATH)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(file), &c)
}

func (c *Config) readFromHttp() error {
	httpc := &http.Client{Timeout: 10 * time.Second}
	r, err := httpc.Get(c.PATH)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	c.l.Infof("Load Config From HTTP: %s", c.PATH)
	return json.NewDecoder(r.Body).Decode(&c)
}

func (c *Config) Adjust() error {
	if c.LogLeveL == "" {
		c.LogLeveL = "info"
	}
	if c.WebHost == "" {
		c.WebHost = "0.0.0.0"
	}

	clashSubList, err := c.GetClashSubList()
	if err != nil {
		return err
	}
	for _, clashSub := range clashSubList {
		if err := clashSub.Refresh(); err != nil {
			return err
		}
		relayConfigs, err := clashSub.ToRelayConfigs(c.WebHost)
		if err != nil {
			return err
		}
		c.RelayConfigs = append(c.RelayConfigs, relayConfigs...)
	}

	for _, r := range c.RelayConfigs {
		if err := r.Validate(); err != nil {
			return err
		}
	}

	// check relay config label is unique
	labelMap := make(map[string]struct{})
	for _, r := range c.RelayConfigs {
		if _, ok := labelMap[r.Label]; ok {
			return fmt.Errorf("relay label %s is not unique", r.Label)
		}
		labelMap[r.Label] = struct{}{}
	}
	return nil
}

func (c *Config) NeedStartWebServer() bool {
	return c.WebPort != 0
}

func (c *Config) NeedStartXrayServer() bool {
	return c.XRayConfig != nil
}

func (c *Config) NeedStartRelayServer() bool {
	return len(c.RelayConfigs) > 0
}

func (c *Config) GetMetricURL() string {
	if !c.NeedStartWebServer() {
		return ""
	}
	url := fmt.Sprintf("http://%s:%d/metrics/", c.WebHost, c.WebPort)
	if c.WebToken != "" {
		url += fmt.Sprintf("?token=%s", c.WebToken)
	}
	return url
}

func (c *Config) GetClashSubList() ([]*sub.ClashSub, error) {
	clashSubList := make([]*sub.ClashSub, 0, len(c.SubConfigs))
	for _, subCfg := range c.SubConfigs {
		clashSub, err := c.getOrCreateClashSub(subCfg)
		if err != nil {
			return nil, err
		}
		clashSubList = append(clashSubList, clashSub)
	}
	return clashSubList, nil
}

func (c *Config) getOrCreateClashSub(subCfg *SubConfig) (*sub.ClashSub, error) {
	if clashSub, ok := c.cachedClashSubMap[subCfg.Name]; ok {
		return clashSub, nil
	}
	clashSub, err := sub.NewClashSubByURL(subCfg.URL, subCfg.Name)
	if err != nil {
		return nil, err
	}
	c.cachedClashSubMap[subCfg.Name] = clashSub
	return clashSub, nil
}

type SubConfig struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}
