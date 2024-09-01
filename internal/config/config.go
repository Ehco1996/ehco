package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/relay/conf"
	"github.com/Ehco1996/ehco/internal/tls"
	myhttp "github.com/Ehco1996/ehco/pkg/http"
	xConf "github.com/xtls/xray-core/infra/conf"
	"go.uber.org/zap"
)

type Config struct {
	PATH string `json:"-"`

	NodeLabel   string `json:"node_label,omitempty"`
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

	XRayConfig          *xConf.Config `json:"xray_config,omitempty"`
	SyncTrafficEndPoint string        `json:"sync_traffic_endpoint,omitempty"`

	lastLoadTime time.Time
	l            *zap.SugaredLogger
}

func NewConfig(path string) *Config {
	return &Config{PATH: path, l: zap.S().Named("cfg")}
}

func (c *Config) NeedSyncFromServer() bool {
	return strings.Contains(c.PATH, "http")
}

func (c *Config) LoadConfig(force bool) error {
	if c.ReloadInterval > 0 && time.Since(c.lastLoadTime).Seconds() < float64(c.ReloadInterval) && !force {
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
	return json.Unmarshal([]byte(file), &c)
}

func (c *Config) readFromHttp() error {
	c.l.Infof("Load Config From HTTP: %s", c.PATH)
	return myhttp.GetJSONWithRetry(c.PATH, &c)
}

func (c *Config) Adjust() error {
	if c.LogLeveL == "" {
		c.LogLeveL = "info"
	}
	if c.WebHost == "" {
		c.WebHost = "0.0.0.0"
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
	// init tls when need
	for _, r := range c.RelayConfigs {
		if r.ListenType == constant.RelayTypeWSS || r.TransportType == constant.RelayTypeWSS {
			if err := tls.InitTlsCfg(); err != nil {
				return err
			}
			break
		}
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
	// for basic auth
	if c.WebAuthUser != "" && c.WebAuthPass != "" {
		url = fmt.Sprintf("http://%s:%s@%s:%d/metrics/", c.WebAuthUser, c.WebAuthPass, c.WebHost, c.WebPort)
	}
	return url
}
