package sub

import (
	"fmt"

	relay_cfg "github.com/Ehco1996/ehco/internal/relay/conf"
	"gopkg.in/yaml.v3"
)

type ClashSub struct {
	Name string
	URL  string

	cCfg *clashConfig
}

func NewClashSub(rawClashCfgBuf []byte, name string, url string) (*ClashSub, error) {
	var rawCfg clashConfig
	err := yaml.Unmarshal(rawClashCfgBuf, &rawCfg)
	if err != nil {
		return nil, err
	}
	rawCfg.Adjust()
	return &ClashSub{cCfg: &rawCfg, Name: name, URL: url}, nil
}

func NewClashSubByURL(url string, name string) (*ClashSub, error) {
	body, err := getHttpBody(url)
	if err != nil {
		return nil, err
	}
	return NewClashSub(body, name, url)
}

func (c *ClashSub) ToClashConfigYaml() ([]byte, error) {
	return yaml.Marshal(c.cCfg)
}

func (c *ClashSub) Refresh() error {
	// get new clash sub by url
	newSub, err := NewClashSubByURL(c.URL, c.Name)
	if err != nil {
		return err
	}

	needAdd := []*Proxies{}
	needDeleteProxyName := map[string]struct{}{}

	// check if need add/delete proxies
	for _, newProxy := range *newSub.cCfg.Proxies {
		oldProxy := c.cCfg.GetProxyByRawName(newProxy.rawName)
		if oldProxy == nil {
			println("need add", newProxy.Name)
			needAdd = append(needAdd, newProxy)
		} else if oldProxy.Different(newProxy) {
			// update  so we need to delete and add again
			needDeleteProxyName[oldProxy.rawName] = struct{}{}
			needAdd = append(needAdd, newProxy)
			println("need update", oldProxy.rawName)
		}
	}
	// check if need delete proxies
	for _, proxy := range *c.cCfg.Proxies {
		newProxy := newSub.cCfg.GetProxyByRawName(proxy.rawName)
		if newProxy == nil {
			needDeleteProxyName[proxy.rawName] = struct{}{}
			println("need delete", proxy.rawName)
		}
	}

	tmp := []*Proxies{}
	// delete proxies from changedCfg
	for _, p := range *c.cCfg.Proxies {
		if _, ok := needDeleteProxyName[p.rawName]; !ok {
			tmp = append(tmp, p)
		}
	}
	// add new proxies to changedCfg
	tmp = append(tmp, needAdd...)

	// update current
	c.cCfg.Proxies = &tmp
	return nil
}

func (c *ClashSub) ToRelayConfigs(listenHost string) ([]*relay_cfg.Config, error) {
	relayConfigs := []*relay_cfg.Config{}
	for _, proxy := range *c.cCfg.Proxies {
		newName := fmt.Sprintf("%s-%s", c.Name, proxy.Name)
		rc, err := proxy.ToRelayConfig(listenHost, newName)
		if err != nil {
			return nil, err
		}
		relayConfigs = append(relayConfigs, rc)
	}
	return relayConfigs, nil
}
