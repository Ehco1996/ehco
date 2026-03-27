package transporter

import (
	"bytes"
	"io"
	"net"

	"github.com/Ehco1996/ehco/internal/relay/conf"
)

const peekBufferSize = 4096

// sniffProtocol detects the protocol from peeked connection data.
// Returns conf.ProtocolTLS, conf.ProtocolHTTP, or empty string if unknown.
func sniffProtocol(data []byte) string {
	if isTLSClientHello(data) {
		return conf.ProtocolTLS
	}
	if isHTTPRequest(data) {
		return conf.ProtocolHTTP
	}
	return ""
}

// isTLSClientHello checks if data looks like a TLS ClientHello message.
// TLS record: ContentType(1) | Version(2) | Length(2) | HandshakeType(1)
// ContentType 0x16 = Handshake, HandshakeType 0x01 = ClientHello
func isTLSClientHello(data []byte) bool {
	if len(data) < 6 {
		return false
	}
	if data[0] != 0x16 {
		return false
	}
	if data[1] != 0x03 || data[2] > 0x03 {
		return false
	}
	return data[5] == 0x01
}

var httpMethods = [][]byte{
	[]byte("GET "),
	[]byte("POST "),
	[]byte("HEAD "),
	[]byte("PUT "),
	[]byte("DELETE "),
	[]byte("OPTIONS "),
	[]byte("PATCH "),
	[]byte("CONNECT "),
	[]byte("TRACE "),
}

func isHTTPRequest(data []byte) bool {
	for _, method := range httpMethods {
		if bytes.HasPrefix(data, method) {
			return true
		}
	}
	return false
}

// peekedConn wraps a net.Conn, prepending previously peeked data to reads.
type peekedConn struct {
	net.Conn
	reader io.Reader
}

func newPeekedConn(c net.Conn, peeked []byte) net.Conn {
	return &peekedConn{
		Conn:   c,
		reader: io.MultiReader(bytes.NewReader(peeked), c),
	}
}

func (c *peekedConn) Read(b []byte) (int, error) {
	return c.reader.Read(b)
}
