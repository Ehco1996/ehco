package conf

import (
	"fmt"
	"time"

	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/lb"
)

const (
	ProtocolHTTP = "http"
	ProtocolTLS  = "tls"
)

type Options struct {
	RemotesChain []lb.Remote `json:"remotes_chain"`

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
		RemotesChain:       make([]lb.Remote, len(o.RemotesChain)),
	}
	copy(opt.BlockedProtocols, o.BlockedProtocols)
	copy(opt.RemotesChain, o.RemotesChain)
	return opt
}

func (o *Options) Validate() error {
	for _, protocol := range o.BlockedProtocols {
		if protocol != ProtocolHTTP && protocol != ProtocolTLS {
			return fmt.Errorf("invalid blocked protocol: %s", protocol)
		}
	}
	for _, chain := range o.RemotesChain {
		if err := chain.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (o *Options) NeedSendHandshakePayload() bool {
	return len(o.RemotesChain) > 0
}

type HandshakePayload struct {
	RemotesChain []lb.Remote `json:"remotes_chain"`
}

func BuildHandshakePayload(opt *Options, wsPath string) *HandshakePayload {
	remotes := make([]lb.Remote, len(opt.RemotesChain))
	for i, remote := range opt.RemotesChain {
		remotes[i] = remote
		if remote.TransportType != constant.RelayTypeRaw {
			remote.WSPath = wsPath
		}
	}
	return &HandshakePayload{RemotesChain: remotes}
}

func (p *HandshakePayload) PopNextRemote() *lb.Remote {
	if len(p.RemotesChain) == 0 {
		return nil
	}
	next := &p.RemotesChain[0]
	p.RemotesChain = p.RemotesChain[1:]
	return next
}

func (p *HandshakePayload) Clone() *HandshakePayload {
	if p == nil {
		return nil
	}
	newPayload := &HandshakePayload{
		RemotesChain: make([]lb.Remote, len(p.RemotesChain)),
	}
	copy(newPayload.RemotesChain, p.RemotesChain)
	return newPayload
}
