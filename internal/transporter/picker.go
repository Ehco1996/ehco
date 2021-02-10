package transporter

import (
	"net"
	"net/http"

	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/lb"
)

// RelayTransporter
type RelayTransporter interface {

	// UDP相关
	GetOrCreateBufferCh(uaddr *net.UDPAddr) *BufferCh
	HandleUDPConn(uaddr *net.UDPAddr, local *net.UDPConn)

	// TCP相关
	HandleTCPConn(c *net.TCPConn) error

	// TUN相关
	HandleWebRequset(w http.ResponseWriter, req *http.Request)
}

func PickTransporter(transType string, tcpLBNodes, udpLBNodes *lb.LBNodes) RelayTransporter {
	raw := Raw{
		TCPNodes:       tcpLBNodes,
		UDPNodes:       udpLBNodes,
		UDPBufferChMap: make(map[string]*BufferCh),
	}
	switch transType {
	case constant.Transport_RAW:
		return &raw
	case constant.Transport_WS:
		return &Ws{raw: raw}
	}
	return nil
}
