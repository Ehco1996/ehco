package web

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

type RelayConfig struct {
	Listen        string `json:"listen"`
	ListenType    string `json:"listen_type"`
	Remote        string `json:"remote"`
	TransportType string `json:"transport_type"`
}

type Config struct {
	PATH    string
	Configs []RelayConfig
}

func NewConfig(path string) *Config {
	return &Config{PATH: path, Configs: []RelayConfig{}}
}

func (c *Config) LoadConfig() error {
	if strings.Contains(c.PATH, "http") {
		return c.readFromHttp()
	}
	return c.readFromFile()
}

func (c *Config) readFromFile() error {
	file, err := ioutil.ReadFile(c.PATH)
	if err != nil {
		return err
	}
	err = json.Unmarshal([]byte(file), &c.Configs)
	if err != nil {
		return err
	}
	fmt.Println("[INFO] load config from file:", c.PATH)
	return nil
}

func (c *Config) readFromHttp() error {
	var myClient = &http.Client{Timeout: 10 * time.Second}
	r, err := myClient.Get(c.PATH)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	json.NewDecoder(r.Body).Decode(&c.Configs)
	fmt.Println("[INFO] load config from http:", c.PATH)
	return nil
}
