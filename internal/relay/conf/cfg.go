package conf

import (
	"errors"
	"fmt"

	"github.com/Ehco1996/ehco/internal/constant"
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
	return &Config{
		Listen:        r.Listen,
		ListenType:    r.ListenType,
		TransportType: r.TransportType,
		TCPRemotes:    r.TCPRemotes,
		UDPRemotes:    r.UDPRemotes,
		Label:         r.Label,
	}
}
