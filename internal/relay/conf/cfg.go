package conf

import (
	"errors"
	"fmt"

	"github.com/Ehco1996/ehco/internal/constant"
	"go.uber.org/zap"
)

type Config struct {
	Listen        string   `json:"listen"`
	ListenType    string   `json:"listen_type"`
	TransportType string   `json:"transport_type"`
	TCPRemotes    []string `json:"tcp_remotes"`
	UDPRemotes    []string `json:"udp_remotes"`

	Label string `json:"label,omitempty"`
}

func (r *Config) Validate() error {
	if r.Adjust() != nil {
		return errors.New("adjust config failed")
	}
	if r.ListenType != constant.Listen_RAW &&
		r.ListenType != constant.Listen_WS &&
		r.ListenType != constant.Listen_WSS &&
		r.ListenType != constant.Listen_MTCP &&
		r.ListenType != constant.Listen_MWSS {
		return fmt.Errorf("invalid listen type:%s", r.ListenType)
	}

	if r.TransportType != constant.Transport_RAW &&
		r.TransportType != constant.Transport_WS &&
		r.TransportType != constant.Transport_WSS &&
		r.TransportType != constant.Transport_MTCP &&
		r.TransportType != constant.Transport_MWSS {
		return fmt.Errorf("invalid transport type:%s", r.ListenType)
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
func (r *Config) defaultLabel() string {
	defaultLabel := fmt.Sprintf("<At=%s Over=%s TCP-To=%s UDP-To=%s Through=%s>",
		r.Listen, r.ListenType, r.TCPRemotes, r.UDPRemotes, r.TransportType)
	return defaultLabel
}

func (r *Config) Adjust() error {
	if r.Label == "" {
		r.Label = r.defaultLabel()
		zap.S().Warnf("label is empty, set default label:%s", r.Label)
	}
	return nil
}
