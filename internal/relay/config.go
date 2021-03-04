package relay

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/Ehco1996/ehco/internal/logger"
)

type RelayConfig struct {
	Listen        string   `json:"listen"`
	ListenType    string   `json:"listen_type"`
	TransportType string   `json:"transport_type"`
	TCPRemotes    []string `json:"tcp_remotes"`
	UDPRemotes    []string `json:"udp_remotes"`
}

type JsonConfig struct {
	Configs []RelayConfig `json:"relay_configs"`
}
type Config struct {
	PATH    string
	Configs []RelayConfig
}

func NewConfigByPath(path string) *Config {
	return &Config{PATH: path, Configs: []RelayConfig{}}
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
	jsonConfig := JsonConfig{}
	err = json.Unmarshal([]byte(file), &jsonConfig)
	if err != nil {
		return err
	}
	c.Configs = jsonConfig.Configs
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
	jsonConfig := JsonConfig{}
	if err := json.NewDecoder(r.Body).Decode(&jsonConfig); err != nil {
		return err
	}
	c.Configs = jsonConfig.Configs
	logger.Info("[cfg] Load Config From http:", c.PATH, c.Configs)
	return nil
}

func (c *Config) GetPingHosts() (hosts []string) {
	for _, cfg := range c.Configs {
		hosts = append(hosts, cfg.TCPRemotes...)
	}
	return
}
