package ehco

import (
	"encoding/json"
	"errors"
	"io/ioutil"
)

// Config relay configs
type Config struct {
	LocalHost   string `json:"local_host"`
	LocalPorts  []int  `json:"local_ports"`
	RemoteHost  string `json:"remote_host"`
	RemotePorts []int  `json:"remote_ports"`
}

// ReadConfigFromPath read file form path
func ReadConfigFromPath(filePath string) (Config, error) {
	file, _ := ioutil.ReadFile(filePath)
	config := Config{}
	if err := json.Unmarshal(file, &config); err != nil {
		return config, err
	}
	if len(config.LocalPorts) != len(config.RemotePorts) {
		return config, errors.New("port map invlid")
	}
	return config, nil
}
