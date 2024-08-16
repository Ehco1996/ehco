package sub

import (
	"net"

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

func (cc *clashConfig) GetProxyByName(name string) *Proxies {
	for _, proxy := range *cc.Proxies {
		if proxy.Name == name {
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
	proxies := cc.Proxies

	proxyNameList := []string{}
	for _, proxy := range *proxies {
		proxyNameList = append(proxyNameList, proxy.Name)
	}
	groupNameMap := groupByLongestCommonPrefix(proxyNameList)

	proxyGroups := make(map[string][]*Proxies)
	for groupName, proxyNames := range groupNameMap {
		for _, proxyName := range proxyNames {
			proxyGroups[groupName] = append(proxyGroups[groupName], cc.GetProxyByName(proxyName))
		}
	}
	return proxyGroups
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

	groupLeader *Proxies
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

func (p *Proxies) ToRelayConfig(listenHost string, listenPort string, newName string) (*relay_cfg.Config, error) {
	if p.relayCfg != nil {
		return p.relayCfg, nil
	}
	remoteAddr := net.JoinHostPort(p.Server, p.Port)
	r := &relay_cfg.Config{
		Label:         newName,
		ListenType:    constant.RelayTypeRaw,
		TransportType: constant.RelayTypeRaw,
		Listen:        net.JoinHostPort(listenHost, listenPort),
		TCPRemotes:    []string{remoteAddr},
	}
	if p.UDP {
		r.Options = &relay_cfg.Options{
			EnableUDP: true,
		}
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	// overwrite name,port,and server by relay
	p.Name = newName
	p.Server = listenHost
	p.Port = listenPort
	p.relayCfg = r
	return r, nil
}

func (p *Proxies) Clone() *Proxies {
	cloned := &Proxies{
		Name:           p.Name,
		Type:           p.Type,
		Server:         p.Server,
		Port:           p.Port,
		Password:       p.Password,
		UDP:            p.UDP,
		Cipher:         p.Cipher,
		ALPN:           p.ALPN,
		SkipCertVerify: p.SkipCertVerify,
		SNI:            p.SNI,
		Network:        p.Network,
		UserName:       p.UserName,
		TLS:            p.TLS,
		UUID:           p.UUID,
		AlterID:        p.AlterID,
		ServerName:     p.ServerName,

		rawName:   p.rawName,
		rawServer: p.rawServer,
		rawPort:   p.rawPort,
	}
	if p.relayCfg != nil {
		cloned.relayCfg = p.relayCfg.Clone()
	}
	return cloned
}

func (p *Proxies) getOrCreateGroupLeader() *Proxies {
	if p.groupLeader != nil {
		return p.groupLeader
	}
	p.groupLeader = p.Clone()
	// reset name,port,and server to raw
	p.groupLeader.Name = p.rawName
	p.groupLeader.Port = p.rawPort
	p.groupLeader.Server = p.rawServer
	p.groupLeader.relayCfg = nil
	return p.groupLeader
}
