package conf

import (
	"fmt"
	"time"

	"github.com/Ehco1996/ehco/internal/constant"
)

const (
	ProtocolHTTP = "http"
	ProtocolTLS  = "tls"
)

type ChianRemote struct {
	Addr          string             `json:"addr"`
	NodeLabel     string             `json:"node_label"`
	TransportType constant.RelayType `json:"transport_type"`
}

func (c *ChianRemote) Validate() error {
	if c.Addr == "" {
		return fmt.Errorf("invalid remote addr: %s", c.Addr)
	}
	if !isValidRelayType(c.TransportType) {
		return fmt.Errorf("invalid transport type: %s", c.TransportType)
	}
	return nil
}

type Options struct {
	RemoteChains []ChianRemote `json:"remote_chains"`

	EnableUDP          bool `json:"enable_udp,omitempty"`
	EnableMultipathTCP bool `json:"enable_multipath_tcp,omitempty"`

	// connection limit
	MaxConnection    int      `json:"max_connection,omitempty"`
	BlockedProtocols []string `json:"blocked_protocols,omitempty"`
	MaxReadRateKbps  int64    `json:"max_read_rate_kbps,omitempty"`

	DialTimeoutSec  int `json:"dial_timeout_sec,omitempty"`
	IdleTimeoutSec  int `json:"idle_timeout_sec,omitempty"`
	ReadTimeoutSec  int `json:"read_timeout_sec,omitempty"`
	SniffTimeoutSec int `json:"sniff_timeout_sec,omitempty"`

	// timeout in duration
	DialTimeout  time.Duration `json:"-"`
	IdleTimeout  time.Duration `json:"-"`
	ReadTimeout  time.Duration `json:"-"`
	SniffTimeout time.Duration `json:"-"`
}

func (o *Options) Clone() *Options {
	opt := &Options{
		EnableUDP:          o.EnableUDP,
		EnableMultipathTCP: o.EnableMultipathTCP,
		MaxConnection:      o.MaxConnection,
		MaxReadRateKbps:    o.MaxReadRateKbps,
		BlockedProtocols:   make([]string, len(o.BlockedProtocols)),
		RemoteChains:       make([]ChianRemote, len(o.BlockedProtocols)),
	}
	copy(opt.BlockedProtocols, o.BlockedProtocols)
	copy(opt.RemoteChains, o.RemoteChains)
	return opt
}

func (o *Options) Validate() error {
	for _, protocol := range o.BlockedProtocols {
		if protocol != ProtocolHTTP && protocol != ProtocolTLS {
			return fmt.Errorf("invalid blocked protocol: %s", protocol)
		}
	}
	seenNode := make(map[string]bool)
	for _, chain := range o.RemoteChains {
		if err := chain.Validate(); err != nil {
			return err
		}
		if seenNode[chain.NodeLabel] {
			return fmt.Errorf("duplicate node label: %s", chain.NodeLabel)
		}
		seenNode[chain.NodeLabel] = true
	}

	return nil
}

func (o *Options) NeedSendHandshakePayload() bool {
	return len(o.RemoteChains) > 0
}

type HandshakePayload struct {
	FinalAddr    string        `json:"final_addr"`
	RemoteChains []ChianRemote `json:"remote_chains"`
}

func BuildHandshakePayload(opt *Options, finalAddr string) *HandshakePayload {
	return &HandshakePayload{FinalAddr: finalAddr, RemoteChains: opt.RemoteChains}
}

func (p *HandshakePayload) RemoveLocalChainAndGetNext(nodeLabel string) (*ChianRemote, error) {
	if len(p.RemoteChains) == 0 {
		return nil, nil
	}
	if p.RemoteChains[0].NodeLabel == nodeLabel {
		p.RemoteChains = p.RemoteChains[1:]
		next := &p.RemoteChains[0]
		return next, nil
	}
	return nil, fmt.Errorf("node label %s not the first chain", nodeLabel)
}
