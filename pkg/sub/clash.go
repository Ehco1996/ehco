package sub

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

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

func (c *ClashSub) ToGroupedClashConfigYaml() ([]byte, error) {
	groupProxy := c.cCfg.groupByLongestCommonPrefix()
	ps := []*Proxies{}
	groupNameList := []string{}
	for groupName := range groupProxy {
		groupNameList = append(groupNameList, groupName)
	}
	sort.Strings(groupNameList)
	for _, groupName := range groupNameList {
		proxies := groupProxy[groupName]
		// only use first proxy will be show in proxy provider, other will be merged into load balance in relay
		p := proxies[0].getOrCreateGroupLeader()
		ps = append(ps, p)
	}
	groupedCfg := &clashConfig{&ps}
	return yaml.Marshal(groupedCfg)
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
			needAdd = append(needAdd, newProxy)
		} else if oldProxy.Different(newProxy) {
			// update  so we need to delete and add again
			needDeleteProxyName[oldProxy.rawName] = struct{}{}
			needAdd = append(needAdd, newProxy)
		}
	}
	// check if need delete proxies
	for _, proxy := range *c.cCfg.Proxies {
		newProxy := newSub.cCfg.GetProxyByRawName(proxy.rawName)
		if newProxy == nil {
			needDeleteProxyName[proxy.rawName] = struct{}{}
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

	// init group leader for each group
	groupProxy := c.cCfg.groupByLongestCommonPrefix()
	for _, proxies := range groupProxy {
		// only use first proxy will be show in proxy provider, other will be merged into load balance in relay
		proxies[0].getOrCreateGroupLeader()
	}
	return nil
}

// ToRelayConfigs convert clash sub to relay configs
// Proxy's port will be used as relay listen port
// Group's proxies will be merged into load balance in relay
// a new free port will be used as relay listen port for each group
func (c *ClashSub) ToRelayConfigs(listenHost string) ([]*relay_cfg.Config, error) {
	relayConfigs := []*relay_cfg.Config{}
	portSet := map[int]struct{}{}

	const minPort = 10000
	const maxPort = 65535
	nextPort := minPort
	getNextAvailablePort := func() int {
		for ; nextPort <= maxPort; nextPort++ {
			if _, occupied := portSet[nextPort]; !occupied {
				portSet[nextPort] = struct{}{}
				defer func() { nextPort++ }()
				return nextPort
			}
		}
		return -1
	}

	// generate relay config for each proxy
	for _, proxy := range *c.cCfg.Proxies {
		proxyPort, err := strconv.Atoi(proxy.Port)
		if err != nil {
			return nil, err
		}
		if _, ok := portSet[proxyPort]; ok {
			proxyPort = getNextAvailablePort()
		}
		var newName string
		if strings.HasSuffix(proxy.Name, "-") {
			newName = fmt.Sprintf("%s%s", proxy.Name, c.Name)
		} else {
			newName = fmt.Sprintf("%s-%s", proxy.Name, c.Name)
		}
		rc, err := proxy.ToRelayConfig(listenHost, strconv.Itoa(proxyPort), newName)
		if err != nil {
			return nil, err
		}
		relayConfigs = append(relayConfigs, rc)
	}

	// generate relay config for each group
	groupProxy := c.cCfg.groupByLongestCommonPrefix()
	for groupName, proxies := range groupProxy {
		groupLeader := proxies[0].getOrCreateGroupLeader()
		var newName string
		if strings.HasSuffix(groupName, "-") {
			newName = fmt.Sprintf("%slb", groupName)
		} else {
			newName = fmt.Sprintf("%s-lb", groupName)
		}
		port := getNextAvailablePort()
		if port == -1 {
			return nil, fmt.Errorf("no available port")
		}
		rc, err := groupLeader.ToRelayConfig(listenHost, strconv.Itoa(port), newName)
		if err != nil {
			return nil, err
		}
		// add other proxies address in group to relay config
		for _, proxy := range proxies[1:] {
			remote := net.JoinHostPort(proxy.rawServer, proxy.rawPort)
			if strInArray(remote, rc.TCPRemotes) {
				continue
			}
			rc.TCPRemotes = append(rc.TCPRemotes, remote)
			if proxy.UDP {
				rc.UDPRemotes = append(rc.UDPRemotes, remote)
			}
		}
		relayConfigs = append(relayConfigs, rc)
	}
	return relayConfigs, nil
}
