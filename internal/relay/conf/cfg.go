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

type Config struct {
	Label string `json:"label,omitempty"`

	Listen        string             `json:"listen"`
	ListenType    constant.RelayType `json:"listen_type"`
	TransportType constant.RelayType `json:"transport_type"`

	Remotes []string `json:"remotes"`
	Options *Options `json:"options,omitempty"`
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
	if r.Listen == "" {
		return fmt.Errorf("invalid listen: %s", r.Listen)
	}
	if err := r.validateType(); err != nil {
		return err
	}
	return r.Options.Validate()
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
	nodeList := make([]*lb.Node, len(r.Remotes))
	for idx, addr := range r.Remotes {
		nodeList[idx] = &lb.Node{Address: addr}
	}
	return lb.NewRoundRobin(nodeList)
}

func (r *Config) GetAllRemotes() []*lb.Node {
	lb := r.ToRemotesLB()
	return lb.GetAll()
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
