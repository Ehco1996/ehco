package test

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/Ehco1996/ehco/internal/config"
	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/relay"
	"github.com/Ehco1996/ehco/internal/tls"
	"github.com/Ehco1996/ehco/pkg/log"
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

	MTCP_LISTEN = "0.0.0.0:1238"
	MTCP_REMOTE = "0.0.0.0:2003"
	MTCP_SERVER = "0.0.0.0:2003"
)

func init() {
	_ = log.InitGlobalLogger("info")
	// Start the new echo server.
	go RunEchoServer(ECHO_HOST, ECHO_PORT)

	// init tls,make linter happy
	_ = tls.InitTlsCfg()

	cfg := config.Config{
		PATH: "",
		RelayConfigs: []config.RelayConfig{
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

			// mtcp
			{
				Listen:        MTCP_LISTEN,
				ListenType:    constant.Listen_RAW,
				TCPRemotes:    []string{MTCP_REMOTE},
				TransportType: constant.Transport_MTCP,
			},
			{
				Listen:        MTCP_SERVER,
				ListenType:    constant.Listen_MTCP,
				TCPRemotes:    []string{ECHO_SERVER},
				TransportType: constant.Transport_RAW,
			},
		},
	}

	for _, c := range cfg.RelayConfigs {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go func(ctx context.Context, c config.RelayConfig) {
			r, err := relay.NewRelay(&c)
			if err != nil {
				log.Logger.Fatal(err)
			}
			log.Logger.Fatal(r.ListenAndServe())
		}(ctx, c)
	}

	// wait for  init
	time.Sleep(time.Second)
}

func TestRelayOverRaw(t *testing.T) {

	msg := []byte("hello")
	// test tcp
	res := SendTcpMsg(msg, RAW_LISTEN)
	if string(res) != string(msg) {
		t.Fatal(res)
	}
	t.Log("test tcp done!")

	// test udp
	res = SendUdpMsg(msg, RAW_LISTEN)
	if string(res) != string(msg) {
		t.Fatal(res)
	}
	t.Log("test udp done!")
}

func TestRelayWithDeadline(t *testing.T) {

	msg := []byte("hello")
	conn, err := net.Dial("tcp", RAW_LISTEN)
	if err != nil {
		log.Logger.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write(msg); err != nil {
		log.Logger.Fatal(err)
	}

	buf := make([]byte, len(msg))
	constant.IdleTimeOut = time.Second // change for test
	time.Sleep(constant.IdleTimeOut)
	_, err = conn.Read(buf)
	if err != nil {
		log.Logger.Fatal("need error here")
	}
}

func TestRelayOverWs(t *testing.T) {
	msg := []byte("hello")
	// test tcp
	res := SendTcpMsg(msg, WS_LISTEN)
	if string(res) != string(msg) {
		t.Fatal(res)
	}
	t.Log("test tcp over ws done!")
}

func TestRelayOverWss(t *testing.T) {
	msg := []byte("hello")
	// test tcp
	res := SendTcpMsg(msg, WSS_LISTEN)
	if string(res) != string(msg) {
		t.Fatal(res)
	}
	t.Log("test tcp over wss done!")
}

func TestRelayOverMwss(t *testing.T) {
	msg := []byte("hello")
	var wg sync.WaitGroup
	testCnt := 10
	wg.Add(testCnt)
	for i := 0; i < testCnt; i++ {
		go func(i int) {
			t.Logf("run no: %d test.", i)
			res := SendTcpMsg(msg, MWSS_LISTEN)
			wg.Done()
			if string(res) != string(msg) {
				t.Log(res)
				panic(1)
			}
		}(i)
	}
	wg.Wait()
	t.Log("test tcp over mwss done!")
}

func TestRelayOverMTCP(t *testing.T) {
	msg := []byte("hello")
	var wg sync.WaitGroup

	testCnt := 5
	wg.Add(testCnt)
	for i := 0; i < testCnt; i++ {
		go func(i int) {
			t.Logf("run no: %d test.", i)
			res := SendTcpMsg(msg, MTCP_LISTEN)
			wg.Done()
			if string(res) != string(msg) {
				t.Log(res)
				panic(1)
			}
		}(i)
	}
	wg.Wait()
	t.Log("test tcp over mtcp done!")
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
