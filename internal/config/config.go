package config

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/Ehco1996/ehco/internal/logger"
	xray "github.com/xtls/xray-core/core"
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
	PATH         string
	WebPort      int           `json:"web_port,omitempty"`
	WebToken     string        `json:"web_token,omitempty"`
	EnablePing   bool          `json:"enable_ping,omitempty"`
	RelayConfigs []RelayConfig `json:"relay_configs"`

	XRayConfig *xray.Config `json:"xray_configs,omitempty"`
}

func NewConfigByPath(path string) *Config {
	return &Config{PATH: path, RelayConfigs: []RelayConfig{}}
}

func (c *Config) LoadConfig() error {
	var err error
	if strings.Contains(c.PATH, "http") {
		err = c.readFromHttp()
	} else {
		err = c.readFromFile()
	}
	return err
}

func (c *Config) readFromFile() error {
	file, err := ioutil.ReadFile(c.PATH)
	if err != nil {
		return err
	}
	err = json.Unmarshal([]byte(file), &c)
	if err != nil {
		return err
	}
	logger.Info("[cfg] Load Config From file:", c.PATH)
	return nil
}

func (c *Config) readFromHttp() error {
	var myClient = &http.Client{Timeout: 10 * time.Second}
	r, err := myClient.Get(c.PATH)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		return err
	}
	logger.Info("[cfg] Load Config From http:", c.PATH, c.RelayConfigs)
	return nil
}
