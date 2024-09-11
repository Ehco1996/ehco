package lb

import (
	"net/url"
	"strings"
	"time"

	"github.com/Ehco1996/ehco/internal/constant"
)

type Node struct {
	Address       string             `json:"address"`
	TransportType constant.RelayType `json:"transport_type"`

	HandShakeDuration time.Duration
}

func (n *Node) Clone() *Node {
	return &Node{
		Address:           n.Address,
		TransportType:     n.TransportType,
		HandShakeDuration: n.HandShakeDuration,
	}
}

// NOTE for (https/ws/wss)://xxx.com -> xxx.com
func (n *Node) GetAddrHost() (string, error) {
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
