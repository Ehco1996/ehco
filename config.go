package ehco

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
)

type RelayConfig struct {
	Listen string `json:"listen"`
	Remote string `remote:"listen"`
}

type Config struct {
	PATH    string
	Configs []RelayConfig
}

func NewConfig(path string) *Config {
	config := &Config{PATH: path, Configs: []RelayConfig{}}
	config.readFromFile()
	return config
}

func (c *Config) readFromFile() {
	file, _ := ioutil.ReadFile(c.PATH)
	_ = json.Unmarshal([]byte(file), &c.Configs)
	fmt.Println("[INFO] load config from file:", c.PATH)
}
