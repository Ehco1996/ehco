package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Ehco1996/ehco/internal/constant"
	xrayCfg "github.com/xtls/xray-core/infra/conf"
	"go.uber.org/zap"
)

type RelayConfig struct {
	Listen        string   `json:"listen"`
	ListenType    string   `json:"listen_type"`
	TransportType string   `json:"transport_type"`
	TCPRemotes    []string `json:"tcp_remotes"`
	UDPRemotes    []string `json:"udp_remotes,omitempty"`
	Label         string   `json:"label"`
}

func (r *RelayConfig) Validate() error {
	if r.ListenType != constant.Listen_RAW &&
		r.ListenType != constant.Listen_WS &&
		r.ListenType != constant.Listen_WSS &&
		r.ListenType != constant.Listen_MTCP &&
		r.ListenType != constant.Listen_MWSS {
		return fmt.Errorf("invalid listen type:%s", r.ListenType)
	}

	if r.TransportType != constant.Transport_RAW &&
		r.TransportType != constant.Transport_WS &&
		r.TransportType != constant.Transport_WSS &&
		r.TransportType != constant.Transport_MTCP &&
		r.TransportType != constant.Transport_MWSS {
		return fmt.Errorf("invalid transport type:%s", r.ListenType)
	}

	if r.Listen == "" {
		return fmt.Errorf("invalid listen:%s", r.Listen)
	}

	for _, addr := range r.TCPRemotes {
		if addr == "" {
			return fmt.Errorf("invalid tcp remote addr:%s", addr)
		}
	}

	for _, addr := range r.UDPRemotes {
		if addr == "" {
			return fmt.Errorf("invalid udp remote addr:%s", addr)
		}
	}

	if len(r.TCPRemotes) == 0 && len(r.UDPRemotes) == 0 {
		return errors.New("both tcp and udp remotes are empty")
	}
	return nil
}

type SubConfig struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	ListenHost string `json:"listen_host"`
}


type Config struct {
	PATH         string
	RelayConfigs []*RelayConfig `json:"relay_configs"`

	WebPort        int    `json:"web_port,omitempty"`
	WebToken       string `json:"web_token,omitempty"`
	LogLeveL       string `json:"log_level,omitempty"`
	EnablePing     bool   `json:"enable_ping,omitempty"`
	ReloadInterval int    `json:"reload_interval,omitempty"`

	SyncTrafficEndPoint string          `json:"sync_traffic_endpoint"`
	SubConfigs          []*SubConfig    `json:"sub_configs,omitempty"`
	XRayConfig          *xrayCfg.Config `json:"xray_config,omitempty"`

	lastLoadTime time.Time
	l            *zap.SugaredLogger
}

func NewConfig(path string) *Config {
	return &Config{PATH: path, RelayConfigs: []*RelayConfig{}, l: zap.S().Named("cfg")}
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
	return c.Validate()
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

func (c *Config) Validate() error {
	// validate relay configs
	for _, r := range c.RelayConfigs {
		if err := r.Validate(); err != nil {
			return err
		}
	}
	if c.LogLeveL == "" {
		c.LogLeveL = "info"
	}
	return nil
}

func (c *Config) NeedStartWebServer() bool {
	return c.WebPort != 0
}

func (c *Config) GetMetricURL() string {
	if !c.NeedStartWebServer() {
		return ""
	}
	url := fmt.Sprintf("http://0.0.0.0:%d/metrics/", c.WebPort)
	if c.WebToken != "" {
		url += fmt.Sprintf("?token=%s", c.WebToken)
	}
	return url
}
