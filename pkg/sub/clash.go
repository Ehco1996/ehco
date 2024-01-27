package sub

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"

	"github.com/Ehco1996/ehco/internal/constant"
	relay_cfg "github.com/Ehco1996/ehco/internal/relay/conf"
	"gopkg.in/yaml.v3"
)

type ClashSub struct {
	Name string

	raw   *ClashConfig
	after *ClashConfig

	relayConfigs []*relay_cfg.Config
}

func NewClashSub(rawClashCfgBuf []byte, name string) (*ClashSub, error) {
	var raw ClashConfig
	err := yaml.Unmarshal(rawClashCfgBuf, &raw)
	if err != nil {
		return nil, err
	}
	return &ClashSub{raw: &raw, Name: name}, nil
}

func NewClashSubByURL(url string, name string) (*ClashSub, error) {
	resp, err := http.Get(url)
	if err != nil {
		msg := fmt.Sprintf("http get sub config url=%s meet err=%v", url, err)
		return nil, fmt.Errorf(msg)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("http get sub config url=%s meet status code=%d", url, resp.StatusCode)
		return nil, fmt.Errorf(msg)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		msg := fmt.Sprintf("read body meet err=%v", err)
		return nil, fmt.Errorf(msg)
	}
	return NewClashSub(body, name)
}

func (c *ClashSub) ToClashConfigYaml() ([]byte, error) {
	return yaml.Marshal(c.after)
}

func (c *ClashSub) ToRelayConfigsWithCache(listenHost string) ([]*relay_cfg.Config, error) {
	if len(c.relayConfigs) > 0 {
		return c.relayConfigs, nil
	}
	// do a copy for raw
	after := c.raw
	c.after = after

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
		c.relayConfigs = append(c.relayConfigs, r)
		c.after.Proxies[idx].Server = listenHost
		c.after.Proxies[idx].Port = strconv.Itoa(listenPort)
		c.after.Proxies[idx].Name = fmt.Sprintf("%s-%s", c.Name, proxy.Name)
	}
	return c.relayConfigs, nil
}
