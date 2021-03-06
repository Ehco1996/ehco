package test

import (
	"testing"
	"time"

	"github.com/Ehco1996/ehco/internal/config"
	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/logger"
	"github.com/Ehco1996/ehco/internal/relay"
	"github.com/Ehco1996/ehco/internal/tls"
)

const (
	ECHO_HOST   = "0.0.0.0"
	ECHO_PORT   = 9002
	ECHO_SERVER = "0.0.0.0:9002"

	RAW_LISTEN = "0.0.0.0:1234"

	WS_LISTEN = "0.0.0.0:1235"
	WS_REMOTE = "ws://0.0.0.0:2000"
	WS_SERVER = "0.0.0.0:2000"

	WSS_LISTEN = "0.0.0.0:1236"
	WSS_REMOTE = "wss://0.0.0.0:2001"
	WSS_SERVER = "0.0.0.0:2001"

	MWSS_LISTEN = "0.0.0.0:1237"
	MWSS_REMOTE = "wss://0.0.0.0:2002"
	MWSS_SERVER = "0.0.0.0:2002"
)

func init() {
	// Start the new echo server.
	go RunEchoServer(ECHO_HOST, ECHO_PORT)

	// init tls
	tls.InitTlsCfg()

	cfg := config.Config{
		PATH: "",
		Configs: []config.RelayConfig{
			// raw cfg
			{
				Listen:        RAW_LISTEN,
				ListenType:    constant.Listen_RAW,
				TCPRemotes:    []string{ECHO_SERVER},
				UDPRemotes:    []string{ECHO_SERVER},
				TransportType: constant.Transport_RAW,
			},

			// ws
			{
				Listen:        WS_LISTEN,
				ListenType:    constant.Listen_RAW,
				TCPRemotes:    []string{WS_REMOTE},
				TransportType: constant.Transport_WS,
			},
			{
				Listen:        WS_SERVER,
				ListenType:    constant.Listen_WS,
				TCPRemotes:    []string{ECHO_SERVER},
				TransportType: constant.Transport_RAW,
			},

			// wss
			{
				Listen:        WSS_LISTEN,
				ListenType:    constant.Listen_RAW,
				TCPRemotes:    []string{WSS_REMOTE},
				TransportType: constant.Transport_WSS,
			},
			{
				Listen:        WSS_SERVER,
				ListenType:    constant.Listen_WSS,
				TCPRemotes:    []string{ECHO_SERVER},
				TransportType: constant.Transport_RAW,
			},

			// mwss
			{
				Listen:        MWSS_LISTEN,
				ListenType:    constant.Listen_RAW,
				TCPRemotes:    []string{MWSS_REMOTE},
				TransportType: constant.Transport_MWSS,
			},
			{
				Listen:        MWSS_SERVER,
				ListenType:    constant.Listen_MWSS,
				TCPRemotes:    []string{ECHO_SERVER},
				TransportType: constant.Transport_RAW,
			},
		},
	}
	ch := make(chan error)
	for _, c := range cfg.Configs {
		go func(c config.RelayConfig) {
			r, err := relay.NewRelay(&c)
			if err != nil {
				logger.Fatal(err)
			}
			ch <- r.ListenAndServe()
		}(c)
	}

	// wait for  init
	time.Sleep(time.Second)
}

func TestRelay(t *testing.T) {

	msg := []byte("hello")
	// test tcp
	res := SendTcpMsg(msg, RAW_LISTEN)
	if string(res) != string(msg) {
		t.Fatal(res)
	}
	t.Log("test tcp down!")

	// test udp
	res = SendUdpMsg(msg, RAW_LISTEN)
	if string(res) != string(msg) {
		t.Fatal(res)
	}
	t.Log("test udp down!")
}

func TestRelayOverWs(t *testing.T) {
	msg := []byte("hello")
	// test tcp
	res := SendTcpMsg(msg, WS_LISTEN)
	if string(res) != string(msg) {
		t.Fatal(res)
	}
	t.Log("test tcp over ws down!")
}

func TestRelayOverWss(t *testing.T) {
	msg := []byte("hello")
	// test tcp
	res := SendTcpMsg(msg, WSS_LISTEN)
	if string(res) != string(msg) {
		t.Fatal(res)
	}
	t.Log("test tcp over ws down!")
}

func TestRelayOverMwss(t *testing.T) {
	msg := []byte("hello")
	// test tcp
	res := SendTcpMsg(msg, MWSS_LISTEN)
	if string(res) != string(msg) {
		t.Fatal(res)
	}
	t.Log("test tcp over ws down!")
}

func BenchmarkTcpRelay(b *testing.B) {
	msg := []byte("hello")
	for i := 0; i <= b.N; i++ {
		res := SendTcpMsg(msg, RAW_LISTEN)
		if string(res) != string(msg) {
			b.Fatal(res)
		}
	}
}
