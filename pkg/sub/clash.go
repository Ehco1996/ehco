package sub

import (
	"fmt"
	"net"
	"strconv"

	"github.com/Ehco1996/ehco/internal/constant"
	relay_cfg "github.com/Ehco1996/ehco/internal/relay/conf"
	"gopkg.in/yaml.v3"
)

type ClashSub struct {
	Name string

	raw          *ClashConfig
	after        *ClashConfig
	relayConfigs []*relay_cfg.Config
}

func NewClashSub(rawClashCfgBuf []byte, name string) (*ClashSub, error) {
	var raw ClashConfig
	err := yaml.Unmarshal(rawClashCfgBuf, &raw)
	if err != nil {
		return nil, err
	}
	// do a copy for raw
	after := raw
	return &ClashSub{raw: &raw, after: &after, Name: name}, nil
}

func (c *ClashSub) ToClashConfigYaml() ([]byte, error) {
	return yaml.Marshal(c.after)
}

func (c *ClashSub) ToRelayConfigs(listenHost string) ([]*relay_cfg.Config, error) {
	if len(c.relayConfigs) > 0 {
		return c.relayConfigs, nil
	}

	relayCfg := make([]*relay_cfg.Config, 0, len(c.raw.Proxies))
	freePorts, err := getFreePortInBatch(listenHost, len(c.raw.Proxies))
	if err != nil {
		return nil, err
	}
	// overwrite port and server by relay
	for idx, proxy := range c.raw.Proxies {
		listenPort := freePorts[idx]
		listenAddr := net.JoinHostPort(listenHost, strconv.Itoa(listenPort))
		remoteAddr := net.JoinHostPort(proxy.Server, proxy.Port)
		r := &relay_cfg.Config{
			Label:         proxy.Name,
			ListenType:    constant.Listen_RAW,
			TransportType: constant.Transport_RAW,
			Listen:        listenAddr,
			TCPRemotes:    []string{remoteAddr},
		}
		if proxy.UDP {
			r.UDPRemotes = []string{remoteAddr}
		}
		if err := r.Validate(); err != nil {
			return nil, err
		}
		relayCfg = append(relayCfg, r)
		c.after.Proxies[idx].Server = listenHost
		c.after.Proxies[idx].Port = strconv.Itoa(listenPort)
		c.after.Proxies[idx].Name = fmt.Sprintf("%s-%s", c.Name, proxy.Name)
	}
	c.relayConfigs = relayCfg
	return relayCfg, nil
}
