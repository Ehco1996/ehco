package test

import (
	"github.com/Ehco1996/ehco/internal/relay"
	"testing"
	"time"
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
	relay.InitTlsCfg()

	cfg := relay.Config{
		PATH: "",
		Configs: []relay.RelayConfig{
			// raw cfg
			relay.RelayConfig{
				Listen:        RAW_LISTEN,
				ListenType:    relay.Listen_RAW,
				Remote:        ECHO_SERVER,
				TransportType: relay.Transport_RAW,
			},

			// ws
			relay.RelayConfig{
				Listen:        WS_LISTEN,
				ListenType:    relay.Listen_RAW,
				Remote:        WS_REMOTE,
				TransportType: relay.Transport_WS,
			},
			relay.RelayConfig{
				Listen:        WS_SERVER,
				ListenType:    relay.Listen_WS,
				Remote:        ECHO_SERVER,
				TransportType: relay.Transport_RAW,
			},

			// wss
			relay.RelayConfig{
				Listen:        WSS_LISTEN,
				ListenType:    relay.Listen_RAW,
				Remote:        WSS_REMOTE,
				TransportType: relay.Transport_WSS,
			},
			relay.RelayConfig{
				Listen:        WSS_SERVER,
				ListenType:    relay.Listen_WSS,
				Remote:        ECHO_SERVER,
				TransportType: relay.Transport_RAW,
			},

			// mwss
			relay.RelayConfig{
				Listen:        MWSS_LISTEN,
				ListenType:    relay.Listen_RAW,
				Remote:        MWSS_REMOTE,
				TransportType: relay.Transport_MWSS,
			},
			relay.RelayConfig{
				Listen:        MWSS_SERVER,
				ListenType:    relay.Listen_MWSS,
				Remote:        ECHO_SERVER,
				TransportType: relay.Transport_RAW,
			},
		},
	}
	ch := make(chan error)
	for _, c := range cfg.Configs {
		go func(c relay.RelayConfig) {
			r, err := relay.NewRelay(c.Listen, c.ListenType, c.Remote, c.TransportType)
			if err != nil {
				relay.Logger.Fatal(err)
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
