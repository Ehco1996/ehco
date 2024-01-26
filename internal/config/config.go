package config

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Ehco1996/ehco/internal/relay/conf"
	xConf "github.com/xtls/xray-core/infra/conf"
	"go.uber.org/zap"
)

type Config struct {
	PATH string

	WebHost  string `json:"web_host,omitempty"`
	WebPort  int    `json:"web_port,omitempty"`
	WebToken string `json:"web_token,omitempty"`

	LogLeveL       string `json:"log_level,omitempty"`
	EnablePing     bool   `json:"enable_ping,omitempty"`
	ReloadInterval int    `json:"reload_interval,omitempty"`

	RelayConfigs        []*conf.Config `json:"relay_configs"`
	XRayConfig          *xConf.Config  `json:"xray_config,omitempty"`
	SyncTrafficEndPoint string         `json:"sync_traffic_endpoint,omitempty"`

	lastLoadTime time.Time
	l            *zap.SugaredLogger
}

func NewConfig(path string) *Config {
	return &Config{PATH: path, l: zap.S().Named("cfg")}
}

func (c *Config) NeedSyncUserFromServer() bool {
	return strings.Contains(c.PATH, "http")
}

func (c *Config) LoadConfig() error {
	if c.ReloadInterval > 0 && time.Since(c.lastLoadTime).Seconds() < float64(c.ReloadInterval) {
		c.l.Debugf("Skip Load Config, last load time: %s", c.lastLoadTime)
		return nil
	}
	c.lastLoadTime = time.Now()
	if c.NeedSyncUserFromServer() {
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
	c.l.Debugf("Load Config From File: %s", c.PATH)
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
	c.l.Debugf("Load Config From HTTP: %s", c.PATH)
	return json.NewDecoder(r.Body).Decode(&c)
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
