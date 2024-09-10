package conf

import (
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/lb"

	"go.uber.org/zap"
)

const (
	ProtocolHTTP = "http"
	ProtocolTLS  = "tls"
)

type Options struct {
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
	}
	copy(opt.BlockedProtocols, o.BlockedProtocols)
	return opt
}

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

type Config struct {
	Label string `json:"label,omitempty"`

	Listen        string             `json:"listen"`
	ListenType    constant.RelayType `json:"listen_type"`
	TransportType constant.RelayType `json:"transport_type"`

	Remotes []string `json:"remotes"`
	Options *Options `json:"options,omitempty"`

	RemoteChains []ChianRemote `json:"remote_chains"`
}

func (r *Config) Adjust() error {
	if r.Label == "" {
		r.Label = r.DefaultLabel()
		zap.S().Debugf("label is empty, set default label:%s", r.Label)
	}

	if r.Options == nil {
		r.Options = newDefaultOptions()
	} else {
		r.Options.DialTimeout = getDuration(r.Options.DialTimeoutSec, constant.DefaultDialTimeOut)
		r.Options.IdleTimeout = getDuration(r.Options.IdleTimeoutSec, constant.DefaultIdleTimeOut)
		r.Options.ReadTimeout = getDuration(r.Options.ReadTimeoutSec, constant.DefaultReadTimeOut)
		r.Options.SniffTimeout = getDuration(r.Options.SniffTimeoutSec, constant.DefaultSniffTimeOut)
	}
	return nil
}

func (r *Config) Validate() error {
	if err := r.Adjust(); err != nil {
		return errors.New("adjust config failed")
	}
	if err := r.validateType(); err != nil {
		return err
	}
	if r.Listen == "" {
		return fmt.Errorf("invalid listen: %s", r.Listen)
	}
	for _, addr := range r.Remotes {
		if addr == "" {
			return fmt.Errorf("invalid remote addr: %s", addr)
		}
	}

	for _, chain := range r.RemoteChains {
		if err := chain.Validate(); err != nil {
			return err
		}
	}
	if len(r.RemoteChains) > 0 {
		first := r.RemoteChains[0]
		if first.TransportType == constant.RelayTypeRaw {
			return fmt.Errorf("invalid remote chain on node: %s, raw transport type not support remote chain", first.NodeLabel)
		}
		last := r.RemoteChains[len(r.RemoteChains)-1]
		if last.TransportType != constant.RelayTypeRaw {
			return fmt.Errorf("invalid remote chain on node: %s, last node must be raw transport type", last.NodeLabel)
		}
	}

	for _, protocol := range r.Options.BlockedProtocols {
		if protocol != ProtocolHTTP && protocol != ProtocolTLS {
			return fmt.Errorf("invalid blocked protocol: %s", protocol)
		}
	}
	return nil
}

func (r *Config) Clone() *Config {
	new := &Config{
		Listen:        r.Listen,
		ListenType:    r.ListenType,
		TransportType: r.TransportType,
		Label:         r.Label,
		Options:       r.Options.Clone(),
	}
	new.Remotes = make([]string, len(r.Remotes))
	copy(new.Remotes, r.Remotes)
	return new
}

func (r *Config) Different(new *Config) bool {
	return !reflect.DeepEqual(r, new)
}

// todo make this shorter and more readable
func (r *Config) DefaultLabel() string {
	defaultLabel := fmt.Sprintf("<At=%s To=%s By=%s>",
		r.Listen, r.Remotes, r.TransportType)
	return defaultLabel
}

func (r *Config) ToRemotesLB() lb.RoundRobin {
	tcpNodeList := make([]*lb.Node, len(r.Remotes))
	for idx, addr := range r.Remotes {
		tcpNodeList[idx] = &lb.Node{Address: addr}
	}
	return lb.NewRoundRobin(tcpNodeList)
}

func (r *Config) GetAllRemotes() []*lb.Node {
	lb := r.ToRemotesLB()
	return lb.GetAll()
}

func (r *Config) GetLoggerName() string {
	return fmt.Sprintf("%s(%s<->%s)", r.Label, r.ListenType, r.TransportType)
}

func (r *Config) validateType() error {
	if !isValidRelayType(r.ListenType) {
		return fmt.Errorf("invalid listen type: %s", r.ListenType)
	}

	if !isValidRelayType(r.TransportType) {
		return fmt.Errorf("invalid transport type: %s", r.TransportType)
	}
	return nil
}

func getDuration(seconds int, defaultDuration time.Duration) time.Duration {
	if seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return defaultDuration
}

func newDefaultOptions() *Options {
	return &Options{
		EnableMultipathTCP: true,

		DialTimeout:    constant.DefaultDialTimeOut,
		DialTimeoutSec: int(constant.DefaultDialTimeOut.Seconds()),

		IdleTimeout:    constant.DefaultIdleTimeOut,
		IdleTimeoutSec: int(constant.DefaultIdleTimeOut.Seconds()),

		ReadTimeout:    constant.DefaultReadTimeOut,
		ReadTimeoutSec: int(constant.DefaultReadTimeOut.Seconds()),

		SniffTimeout:    constant.DefaultSniffTimeOut,
		SniffTimeoutSec: int(constant.DefaultSniffTimeOut.Seconds()),
	}
}

func isValidRelayType(rt constant.RelayType) bool {
	return rt == constant.RelayTypeRaw ||
		rt == constant.RelayTypeWS ||
		rt == constant.RelayTypeWSS
}
