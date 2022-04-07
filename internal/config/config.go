package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/logger"
	"github.com/xtls/xray-core/infra/conf"
)

type RelayConfig struct {
	Listen        string   `json:"listen"`
	ListenType    string   `json:"listen_type"`
	TransportType string   `json:"transport_type"`
	TCPRemotes    []string `json:"tcp_remotes"`
	UDPRemotes    []string `json:"udp_remotes"`
	Label         string   `json:"label"`
}

func (r *RelayConfig) Validate() error {
	if r.ListenType != constant.Listen_RAW &&
		r.ListenType != constant.Listen_WS &&
		r.ListenType != constant.Listen_WSS &&
		r.ListenType != constant.Listen_MWSS {
		return fmt.Errorf("invalid listen type:%s", r.ListenType)
	}

	if r.TransportType != constant.Transport_RAW &&
		r.TransportType != constant.Transport_WS &&
		r.TransportType != constant.Transport_WSS &&
		r.TransportType != constant.Transport_MWSS {
		return fmt.Errorf("invalid transport type:%s", r.ListenType)
	}

	if r.Listen == "" {
		return fmt.Errorf("invalid listen:%s", r.Listen)
	}

	if len(r.TCPRemotes) == 0 {
		return fmt.Errorf("invalid remote:%s", r.TCPRemotes)
	}
	return nil
}

type Config struct {
	PATH string

	WebPort    int    `json:"web_port,omitempty"`
	WebToken   string `json:"web_token,omitempty"`
	EnablePing bool   `json:"enable_ping,omitempty"`

	RelayConfigs        []RelayConfig `json:"relay_configs"`
	XRayConfig          *conf.Config  `json:"xray_config,omitempty"`
	SyncTrafficEndPoint string        `json:"sync_traffic_endpoint"`
}

func NewConfigByPath(path string) *Config {
	return &Config{PATH: path, RelayConfigs: []RelayConfig{}}
}

func (c *Config) NeedSyncUserFromServer() bool {
	return strings.Contains(c.PATH, "http")
}

func (c *Config) LoadConfig() error {
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
	file, err := ioutil.ReadFile(c.PATH)
	if err != nil {
		return err
	}
	logger.Info("[cfg] Load Config From file: ", c.PATH)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(file), &c)
}

func (c *Config) readFromHttp() error {
	var httpc = &http.Client{Timeout: 10 * time.Second}
	r, err := httpc.Get(c.PATH)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	logger.Info("[cfg] Load Config From http:", c.PATH)
	return json.NewDecoder(r.Body).Decode(&c)
}

func (c *Config) Validate() error {
	// validate relay configs
	for _, r := range c.RelayConfigs {
		if err := r.Validate(); err != nil {
			return err
		}
	}
	return nil
}
