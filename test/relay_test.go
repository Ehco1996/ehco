package test

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/Ehco1996/ehco/internal/cmgr"
	"github.com/Ehco1996/ehco/internal/config"
	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/relay"
	"github.com/Ehco1996/ehco/internal/relay/conf"
	"github.com/Ehco1996/ehco/internal/tls"
	"github.com/Ehco1996/ehco/pkg/log"
	"github.com/Ehco1996/ehco/test/echo"
	"go.uber.org/zap"
)

const (
	ECHO_HOST   = "0.0.0.0"
	ECHO_PORT   = 9002
	ECHO_SERVER = "0.0.0.0:9002"

	RAW_LISTEN                     = "0.0.0.0:1234"
	RAW_LISTEN_WITH_MAX_CONNECTION = "0.0.0.0:2234"

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
	_ = log.InitGlobalLogger("debug")
	// Start the new echo server.
	go echo.RunEchoServer(ECHO_HOST, ECHO_PORT)

	// init tls,make linter happy
	_ = tls.InitTlsCfg()

	cfg := config.Config{
		PATH: "",
		RelayConfigs: []*conf.Config{
			// raw cfg
			{
				Listen:        RAW_LISTEN,
				ListenType:    constant.RelayTypeRaw,
				TCPRemotes:    []string{ECHO_SERVER},
				UDPRemotes:    []string{ECHO_SERVER},
				TransportType: constant.RelayTypeRaw,
			},
			// raw cfg with max connection
			{
				Listen:        RAW_LISTEN_WITH_MAX_CONNECTION,
				ListenType:    constant.RelayTypeRaw,
				TCPRemotes:    []string{ECHO_SERVER},
				UDPRemotes:    []string{ECHO_SERVER},
				TransportType: constant.RelayTypeRaw,
				MaxConnection: 1,
			},

			// ws
			{
				Listen:        WS_LISTEN,
				ListenType:    constant.RelayTypeRaw,
				TCPRemotes:    []string{WS_REMOTE},
				TransportType: constant.RelayTypeWS,
			},
			{
				Listen:        WS_SERVER,
				ListenType:    constant.RelayTypeWS,
				TCPRemotes:    []string{ECHO_SERVER},
				TransportType: constant.RelayTypeRaw,
			},

			// wss
			{
				Listen:        WSS_LISTEN,
				ListenType:    constant.RelayTypeRaw,
				TCPRemotes:    []string{WSS_REMOTE},
				TransportType: constant.RelayTypeWSS,
			},
			{
				Listen:        WSS_SERVER,
				ListenType:    constant.RelayTypeWSS,
				TCPRemotes:    []string{ECHO_SERVER},
				TransportType: constant.RelayTypeRaw,
			},

			// mwss
			{
				Listen:        MWSS_LISTEN,
				ListenType:    constant.RelayTypeRaw,
				TCPRemotes:    []string{MWSS_REMOTE},
				TransportType: constant.RelayTypeMWSS,
			},
			{
				Listen:        MWSS_SERVER,
				ListenType:    constant.RelayTypeMWSS,
				TCPRemotes:    []string{ECHO_SERVER},
				TransportType: constant.RelayTypeRaw,
			},

			// mtcp
			{
				Listen:        MTCP_LISTEN,
				ListenType:    constant.RelayTypeRaw,
				TCPRemotes:    []string{MTCP_REMOTE},
				TransportType: constant.RelayTypeMTCP,
			},
			{
				Listen:        MTCP_SERVER,
				ListenType:    constant.RelayTypeMTCP,
				TCPRemotes:    []string{ECHO_SERVER},
				TransportType: constant.RelayTypeRaw,
			},
		},
	}
	logger := zap.S()

	for _, c := range cfg.RelayConfigs {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go func(ctx context.Context, c *conf.Config) {
			r, err := relay.NewRelay(c, cmgr.NewCmgr(cmgr.DummyConfig))
			if err != nil {
				logger.Fatal(err)
			}
			logger.Fatal(r.ListenAndServe())
		}(ctx, c)
	}

	// wait for  init
	time.Sleep(time.Second)
}

func TestRelayOverRaw(t *testing.T) {
	msg := []byte("hello")
	// test tcp
	res := echo.SendTcpMsg(msg, RAW_LISTEN)
	if string(res) != string(msg) {
		t.Fatal(res)
	}
	t.Log("test tcp done!")

	// test udp
	// res = echo.SendUdpMsg(msg, RAW_LISTEN)
	// if string(res) != string(msg) {
	// 	t.Fatal(res)
	// }
	// t.Log("test udp done!")
}

func TestRelayWithMaxConnectionCount(t *testing.T) {
	msg := []byte("hello")

	// first connection will be accepted
	go func() {
		err := echo.EchoTcpMsgLong(msg, time.Second, RAW_LISTEN_WITH_MAX_CONNECTION)
		if err != nil {
			t.Error(err)
		}
	}()

	// second connection will be rejected
	time.Sleep(time.Second) // wait for first connection
	if err := echo.EchoTcpMsgLong(msg, time.Second, RAW_LISTEN_WITH_MAX_CONNECTION); err == nil {
		t.Fatal("need error here")
	}
}

func TestRelayWithDeadline(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	msg := []byte("hello")
	conn, err := net.Dial("tcp", RAW_LISTEN)
	if err != nil {
		logger.Sugar().Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write(msg); err != nil {
		logger.Sugar().Fatal(err)
	}

	buf := make([]byte, len(msg))
	constant.IdleTimeOut = time.Second // change for test
	time.Sleep(constant.IdleTimeOut)
	_, err = conn.Read(buf)
	if err != nil {
		logger.Sugar().Fatal("need error here")
	}
}

func TestRelayOverWs(t *testing.T) {
	msg := []byte("hello")
	// test tcp
	res := echo.SendTcpMsg(msg, WS_LISTEN)
	if string(res) != string(msg) {
		t.Fatal(res)
	}
	t.Log("test tcp over ws done!")
}

func TestRelayOverWss(t *testing.T) {
	msg := []byte("hello")
	// test tcp
	res := echo.SendTcpMsg(msg, WSS_LISTEN)
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
			res := echo.SendTcpMsg(msg, MWSS_LISTEN)
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
			res := echo.SendTcpMsg(msg, MTCP_LISTEN)
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
		res := echo.SendTcpMsg(msg, RAW_LISTEN)
		if string(res) != string(msg) {
			b.Fatal(res)
		}
	}
}
