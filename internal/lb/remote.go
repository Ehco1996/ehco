package lb

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Ehco1996/ehco/internal/constant"
)

type Remote struct {
	Address       string             `json:"address"`
	TransportType constant.RelayType `json:"transport_type"`

	HandShakeDuration time.Duration
}

func (n *Remote) Clone() *Remote {
	return &Remote{
		Address:           n.Address,
		TransportType:     n.TransportType,
		HandShakeDuration: n.HandShakeDuration,
	}
}

func (n *Remote) Validate() error {
	if n.Address == "" {
		return fmt.Errorf("invalid address: %s", n.Address)
	}
	return nil
}

// NOTE for (https/ws/wss)://xxx.com -> xxx.com
func (n *Remote) GetAddrHost() (string, error) {
	return extractHost(n.Address)
}

func extractHost(input string) (string, error) {
	// Check if the input string has a scheme, if not, add "http://"
	if !strings.Contains(input, "://") {
		input = "http://" + input
	}
	// Parse the URL
	u, err := url.Parse(input)
	if err != nil {
		return "", err
	}
	return u.Hostname(), nil
}
