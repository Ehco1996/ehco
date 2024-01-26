package sub

import (
	"fmt"
	"net"
	"strconv"

	"gopkg.in/yaml.v3"

	"github.com/Ehco1996/ehco/internal/config"
	"github.com/Ehco1996/ehco/internal/constant"
)

type ClashSub struct {
	raw *ClashConfig
}

func NewClashSub(rawClashCfgBuf []byte) (*ClashSub, error) {
	var raw ClashConfig
	err := yaml.Unmarshal(rawClashCfgBuf, &raw)
	if err != nil {
		return nil, err
	}
	return &ClashSub{raw: &raw}, nil
}

func (c *ClashSub) ToClashConfigYaml() ([]byte, error) {
	return yaml.Marshal(c.raw)
}

func (c *ClashSub) ToRelayConfigs(listenHost string) ([]*config.RelayConfig, error) {
	relayCfg := make([]*config.RelayConfig, 0, len(c.raw.Proxies))
	freePorts, err := getFreePortInBatch(listenHost, len(c.raw.Proxies))
	if err != nil {
		return nil, err
	}
	for idx, proxy := range c.raw.Proxies {

		listenPort := freePorts[idx]
		listenAddr := net.JoinHostPort(listenHost, strconv.Itoa(listenPort))
		remoteAddr := net.JoinHostPort(proxy.Server, strconv.Itoa(proxy.Port))
		r := &config.RelayConfig{
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
	}
	return relayCfg, nil
}

func getFreePortInBatch(host string, count int) ([]int, error) {
	res := make([]int, 0, count)
	listenerList := make([]net.Listener, 0, count)
	for i := 0; i < count; i++ {
		listener, err := net.Listen("tcp", fmt.Sprintf("%s:0", host))
		if err != nil {
			return res, err
		}
		listenerList = append(listenerList, listener)
		address := listener.Addr().(*net.TCPAddr)
		res = append(res, address.Port)
	}
	for _, listener := range listenerList {
		listener.Close()
	}
	return res, nil
}
