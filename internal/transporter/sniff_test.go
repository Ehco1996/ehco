package transporter

import (
	"testing"

	"github.com/Ehco1996/ehco/internal/relay/conf"
	"github.com/stretchr/testify/assert"
)

func TestIsTLSClientHello(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{
			name: "valid TLS 1.0 ClientHello",
			data: []byte{0x16, 0x03, 0x01, 0x00, 0x05, 0x01},
			want: true,
		},
		{
			name: "valid TLS 1.2 ClientHello",
			data: []byte{0x16, 0x03, 0x03, 0x00, 0xf1, 0x01, 0x00, 0x00, 0xed},
			want: true,
		},
		{
			name: "TLS handshake but not ClientHello (ServerHello=0x02)",
			data: []byte{0x16, 0x03, 0x03, 0x00, 0x05, 0x02},
			want: false,
		},
		{
			name: "not TLS - wrong content type",
			data: []byte{0x15, 0x03, 0x01, 0x00, 0x05, 0x01},
			want: false,
		},
		{
			name: "not TLS - wrong major version",
			data: []byte{0x16, 0x04, 0x01, 0x00, 0x05, 0x01},
			want: false,
		},
		{
			name: "too short",
			data: []byte{0x16, 0x03, 0x01},
			want: false,
		},
		{
			name: "empty",
			data: []byte{},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isTLSClientHello(tt.data))
		})
	}
}

func TestIsHTTPRequest(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{name: "GET", data: []byte("GET / HTTP/1.1\r\n"), want: true},
		{name: "POST", data: []byte("POST /api HTTP/1.1\r\n"), want: true},
		{name: "HEAD", data: []byte("HEAD / HTTP/1.1\r\n"), want: true},
		{name: "PUT", data: []byte("PUT /data HTTP/1.1\r\n"), want: true},
		{name: "DELETE", data: []byte("DELETE /item HTTP/1.1\r\n"), want: true},
		{name: "OPTIONS", data: []byte("OPTIONS * HTTP/1.1\r\n"), want: true},
		{name: "PATCH", data: []byte("PATCH /res HTTP/1.1\r\n"), want: true},
		{name: "CONNECT", data: []byte("CONNECT host:443 HTTP/1.1\r\n"), want: true},
		{name: "TRACE", data: []byte("TRACE / HTTP/1.1\r\n"), want: true},
		{name: "not HTTP - TLS", data: []byte{0x16, 0x03, 0x01}, want: false},
		{name: "not HTTP - random", data: []byte("HELLO world"), want: false},
		{name: "empty", data: []byte{}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isHTTPRequest(tt.data))
		})
	}
}

func TestSniffProtocol(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{
			name: "TLS",
			data: []byte{0x16, 0x03, 0x03, 0x00, 0xf1, 0x01},
			want: conf.ProtocolTLS,
		},
		{
			name: "HTTP",
			data: []byte("GET / HTTP/1.1\r\nHost: example.com\r\n"),
			want: conf.ProtocolHTTP,
		},
		{
			name: "unknown",
			data: []byte{0x00, 0x01, 0x02, 0x03},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, sniffProtocol(tt.data))
		})
	}
}
