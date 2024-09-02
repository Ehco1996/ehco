package conf

import (
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/lb"

	"go.uber.org/zap"
)

const (
	ProtocolHTTP         = "http"
	ProtocolTLS          = "tls"
	WS_HANDSHAKE_PATH    = "handshake"
	WS_QUERY_REMOTE_ADDR = "remote_addr"
)

type WSConfig struct {
	Path       string `json:"path,omitempty"`
	RemoteAddr string `json:"remote_addr,omitempty"`
}

func (w *WSConfig) Clone() *WSConfig {
	return &WSConfig{
		Path:       w.Path,
		RemoteAddr: w.RemoteAddr,
	}
}

type Options struct {
	EnableUDP          bool `json:"enable_udp,omitempty"`
	EnableMultipathTCP bool `json:"enable_multipath_tcp,omitempty"`

	// connection limit
	MaxConnection    int      `json:"max_connection,omitempty"`
	BlockedProtocols []string `json:"blocked_protocols,omitempty"`
	MaxReadRateKbps  int64    `json:"max_read_rate_kbps,omitempty"`

	// ws related
	WSConfig *WSConfig `json:"ws_config,omitempty"`

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
	if o.WSConfig != nil {
		opt.WSConfig = o.WSConfig.Clone()
	}
	return opt
}

type Config struct {
	Label         string             `json:"label,omitempty"`
	Listen        string             `json:"listen"`
	ListenType    constant.RelayType `json:"listen_type"`
	TransportType constant.RelayType `json:"transport_type"`
	Remotes       []string           `json:"remotes"`

	Options *Options `json:"options,omitempty"`
}

func (r *Config) GetWSHandShakePath() string {
	if r.Options != nil && r.Options.WSConfig != nil && r.Options.WSConfig.Path != "" {
		return r.Options.WSConfig.Path
	}
	return WS_HANDSHAKE_PATH
}

func (r *Config) GetWSRemoteAddr(baseAddr string) (string, error) {
	addr, err := url.JoinPath(baseAddr, r.GetWSHandShakePath())
	if err != nil {
		return "", err
	}
	if r.Options != nil && r.Options.WSConfig != nil && r.Options.WSConfig.RemoteAddr != "" {
		addr += fmt.Sprintf("?%s=%s", WS_QUERY_REMOTE_ADDR, r.Options.WSConfig.RemoteAddr)
	}
	return addr, nil
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
	if r.Listen != new.Listen ||
		r.ListenType != new.ListenType ||
		r.TransportType != new.TransportType ||
		r.Label != new.Label {
		return true
	}
	if len(r.Remotes) != len(new.Remotes) {
		return true
	}
	for i, addr := range r.Remotes {
		if addr != new.Remotes[i] {
			return true
		}
	}
	return false
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
	if r.ListenType != constant.RelayTypeRaw &&
		r.ListenType != constant.RelayTypeWS &&
		r.ListenType != constant.RelayTypeWSS {
		return fmt.Errorf("invalid listen type:%s", r.ListenType)
	}

	if r.TransportType != constant.RelayTypeRaw &&
		r.TransportType != constant.RelayTypeWS &&
		r.TransportType != constant.RelayTypeWSS {
		return fmt.Errorf("invalid transport type:%s", r.TransportType)
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
