package sub

import (
	"net"
	"strconv"

	"github.com/Ehco1996/ehco/internal/constant"
	relay_cfg "github.com/Ehco1996/ehco/internal/relay/conf"
)

type clashConfig struct {
	Proxies *[]*Proxies `yaml:"proxies"`
}

func (cc *clashConfig) GetProxyByRawName(name string) *Proxies {
	for _, proxy := range *cc.Proxies {
		if proxy.rawName == name {
			return proxy
		}
	}
	return nil
}

func (cc *clashConfig) Adjust() {
	for _, proxy := range *cc.Proxies {
		if proxy.rawName == "" {
			proxy.rawName = proxy.Name
			proxy.rawPort = proxy.Port
			proxy.rawServer = proxy.Server
		}
	}
}

func (cc *clashConfig) groupByLongestCommonPrefix() map[string][]*Proxies {
	proxies := *cc.Proxies
	groups := make(map[string][]*Proxies)

	// 用于记录每个代理是否已分组
	visited := make([]bool, len(proxies))

	for i, currentProxy := range proxies {
		if visited[i] {
			continue
		}
		// 标记当前项为已访问
		visited[i] = true
		// 创建当前代理的分组，至少包含它自身
		group := []*Proxies{currentProxy}
		// 将当前代理的名称设为分组的键
		prefix := currentProxy.Name

		for j, otherProxy := range proxies[i+1:] {
			if visited[i+j+1] {
				continue
			}
			// 找出和当前代理名称的最长公共前缀
			commonPrefix := commonPrefix(prefix, otherProxy.Name)
			// 如果当前代理名称是其他代理名称的前缀，将其添加到当前分组
			if len(commonPrefix) > 0 {
				group = append(group, otherProxy)
				visited[i+j+1] = true
				// 更新前缀以更具体地反映当前分组的公共部分
				if len(commonPrefix) < len(prefix) {
					prefix = commonPrefix
				}
			}
		}

		// 处理完当前代理后，将分组添加到结果中
		if len(prefix) > 0 {
			groups[prefix] = group
		}
	}

	return groups
}

type Proxies struct {
	// basic fields
	Name     string `yaml:"name"`
	Type     string `yaml:"type"`
	Server   string `yaml:"server"`
	Port     string `yaml:"port"`
	Password string `yaml:"password,omitempty"`
	UDP      bool   `yaml:"udp,omitempty"`

	// for shadowsocks todo(support opts)
	Cipher string `yaml:"cipher,omitempty"`

	// for trojan todo(support opts)
	ALPN           []string `yaml:"alpn,omitempty"`
	SkipCertVerify bool     `yaml:"skip-cert-verify,omitempty"`
	SNI            string   `yaml:"sni,omitempty"`
	Network        string   `yaml:"network,omitempty"`

	// for socks5 todo(support opts)
	UserName string `yaml:"username,omitempty"`
	TLS      bool   `yaml:"tls,omitempty"`

	// for vmess todo(support opts)
	UUID       string `yaml:"uuid,omitempty"`
	AlterID    int    `yaml:"alterId,omitempty"`
	ServerName string `yaml:"servername,omitempty"`

	rawName   string
	rawServer string
	rawPort   string
	relayCfg  *relay_cfg.Config
}

func (p *Proxies) Different(new *Proxies) bool {
	if p.Type != new.Type ||
		p.Password != new.Password ||
		p.UDP != new.UDP ||
		p.Cipher != new.Cipher ||
		len(p.ALPN) != len(new.ALPN) ||
		p.SkipCertVerify != new.SkipCertVerify ||
		p.SNI != new.SNI ||
		p.Network != new.Network ||
		p.UserName != new.UserName ||
		p.TLS != new.TLS ||
		p.UUID != new.UUID ||
		p.AlterID != new.AlterID ||
		p.ServerName != new.ServerName {
		return true
	}
	// ALPN field is a slice, should assert values successively.
	for i, v := range p.ALPN {
		if v != new.ALPN[i] {
			return true
		}
	}

	// Server Port Name will be changed when ToRelayConfig is called. so we just need to compare the other fields.
	if p.rawName != new.rawName ||
		p.rawServer != new.rawServer ||
		p.rawPort != new.rawPort {
		return true
	}

	// All fields are equivalent, so proxies are not different.
	return false
}

func (p *Proxies) ToRelayConfig(listenHost string, newName string) (*relay_cfg.Config, error) {
	if p.relayCfg != nil {
		return p.relayCfg, nil
	}
	freePorts, err := getFreePortInBatch(listenHost, 1)
	if err != nil {
		return nil, err
	}
	listenPort := freePorts[0]
	listenAddr := net.JoinHostPort(listenHost, strconv.Itoa(listenPort))
	remoteAddr := net.JoinHostPort(p.Server, p.Port)
	r := &relay_cfg.Config{
		Label:         p.Name,
		ListenType:    constant.Listen_RAW,
		TransportType: constant.Transport_RAW,
		Listen:        listenAddr,
		TCPRemotes:    []string{remoteAddr},
	}
	if p.UDP {
		r.UDPRemotes = []string{remoteAddr}
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	// overwrite name,port,and server by relay
	p.Name = newName
	p.Server = listenHost
	p.Port = strconv.Itoa(listenPort)
	p.relayCfg = r
	return r, nil
}
