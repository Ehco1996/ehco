package conf

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/Ehco1996/ehco/internal/constant"

	"github.com/Ehco1996/ehco/pkg/lb"
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

type Config struct {
	Label         string             `json:"label,omitempty"`
	Listen        string             `json:"listen"`
	ListenType    constant.RelayType `json:"listen_type"`
	TransportType constant.RelayType `json:"transport_type"`
	TCPRemotes    []string           `json:"tcp_remotes"`
	UDPRemotes    []string           `json:"udp_remotes"`

	MaxConnection    int      `json:"max_connection,omitempty"`
	BlockedProtocols []string `json:"blocked_protocols,omitempty"`

	WSConfig *WSConfig `json:"ws_config,omitempty"`
}

func (r *Config) GetWSHandShakePath() string {
	if r.WSConfig != nil && r.WSConfig.Path != "" {
		return r.WSConfig.Path
	}
	return WS_HANDSHAKE_PATH
}

func (r *Config) GetWSRemoteAddr(baseAddr string) (string, error) {
	addr, err := url.JoinPath(baseAddr, r.GetWSHandShakePath())
	if err != nil {
		return "", err
	}
	if r.WSConfig != nil && r.WSConfig.RemoteAddr != "" {
		addr += fmt.Sprintf("?%s=%s", WS_QUERY_REMOTE_ADDR, r.WSConfig.RemoteAddr)
	}
	return addr, nil
}

func (r *Config) Validate() error {
	if r.Adjust() != nil {
		return errors.New("adjust config failed")
	}

	if err := r.validateType(); err != nil {
		return err
	}

	if r.Listen == "" {
		return fmt.Errorf("invalid listen:%s", r.Listen)
	}

	for _, addr := range r.TCPRemotes {
		if addr == "" {
			return fmt.Errorf("invalid tcp remote addr:%s", addr)
		}
	}

	for _, addr := range r.UDPRemotes {
		if addr == "" {
			return fmt.Errorf("invalid udp remote addr:%s", addr)
		}
	}

	if len(r.TCPRemotes) == 0 && len(r.UDPRemotes) == 0 {
		return errors.New("both tcp and udp remotes are empty")
	}

	if len(r.UDPRemotes) > 0 {
		zap.S().Warn("UDP RELAY WAS DISABLED FOR NOW, THIS FEATURE WILL BE AVAILABLE IN THE FUTURE")
	}

	for _, protocol := range r.BlockedProtocols {
		if protocol != ProtocolHTTP && protocol != ProtocolTLS {
			return fmt.Errorf("invalid blocked protocol:%s", protocol)
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
	}
	new.TCPRemotes = make([]string, len(r.TCPRemotes))
	copy(new.TCPRemotes, r.TCPRemotes)
	new.UDPRemotes = make([]string, len(r.UDPRemotes))
	copy(new.UDPRemotes, r.UDPRemotes)
	return new
}

func (r *Config) Different(new *Config) bool {
	if r.Listen != new.Listen ||
		r.ListenType != new.ListenType ||
		r.TransportType != new.TransportType ||
		r.Label != new.Label {
		return true
	}
	if len(r.TCPRemotes) != len(new.TCPRemotes) {
		return true
	}

	for i, addr := range r.TCPRemotes {
		if addr != new.TCPRemotes[i] {
			return true
		}
	}
	if len(r.UDPRemotes) != len(new.UDPRemotes) {
		return true
	}
	for i, addr := range r.UDPRemotes {
		if addr != new.UDPRemotes[i] {
			return true
		}
	}
	return false
}

// todo make this shorter and more readable
func (r *Config) DefaultLabel() string {
	defaultLabel := fmt.Sprintf("<At=%s TCP-To=%s TP=%s>",
		r.Listen, r.TCPRemotes, r.TransportType)
	return defaultLabel
}

func (r *Config) Adjust() error {
	if r.Label == "" {
		r.Label = r.DefaultLabel()
		zap.S().Debugf("label is empty, set default label:%s", r.Label)
	}
	return nil
}

func (r *Config) ToTCPRemotes() lb.RoundRobin {
	tcpNodeList := make([]*lb.Node, len(r.TCPRemotes))
	for idx, addr := range r.TCPRemotes {
		tcpNodeList[idx] = &lb.Node{
			Address: addr,
			Label:   fmt.Sprintf("%s-%s", r.Label, addr),
		}
	}
	return lb.NewRoundRobin(tcpNodeList)
}

func (r *Config) GetLoggerName() string {
	return fmt.Sprintf("%s(%s<->%s)", r.Label, r.ListenType, r.TransportType)
}

func (r *Config) validateType() error {
	if r.ListenType != constant.RelayTypeRaw &&
		r.ListenType != constant.RelayTypeWS &&
		r.ListenType != constant.RelayTypeMWS &&
		r.ListenType != constant.RelayTypeWSS &&
		r.ListenType != constant.RelayTypeMTCP &&
		r.ListenType != constant.RelayTypeMWSS {
		return fmt.Errorf("invalid listen type:%s", r.ListenType)
	}

	if r.TransportType != constant.RelayTypeRaw &&
		r.TransportType != constant.RelayTypeWS &&
		r.TransportType != constant.RelayTypeMWS &&
		r.TransportType != constant.RelayTypeWSS &&
		r.TransportType != constant.RelayTypeMTCP &&
		r.TransportType != constant.RelayTypeMWSS {
		return fmt.Errorf("invalid transport type:%s", r.ListenType)
	}
	return nil
}
