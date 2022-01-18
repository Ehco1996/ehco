package config

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

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
		return c.readFromHttp()
	}
	return c.readFromFile()
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
