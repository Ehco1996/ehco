package test

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
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
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
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

	MWS_LISTEN = "0.0.0.0:1239"
	MWS_REMOTE = "ws://0.0.0.0:2004"
	MSS_SERVER = "0.0.0.0:2004"
)

func TestMain(m *testing.M) {
	// Setup
	_ = log.InitGlobalLogger("debug")
	_ = tls.InitTlsCfg()

	// Start echo server
	echoServer := echo.NewEchoServer(ECHO_HOST, ECHO_PORT)
	go echoServer.Run()

	// Start relay servers
	relayServers := startRelayServers()

	// Run tests
	code := m.Run()

	// Cleanup
	echoServer.Stop()
	for _, server := range relayServers {
		server.Close()
	}

	os.Exit(code)
}

func startRelayServers() []*relay.Relay {
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

			// mws
			{
				Listen:        MWS_LISTEN,
				ListenType:    constant.RelayTypeRaw,
				TCPRemotes:    []string{MWS_REMOTE},
				TransportType: constant.RelayTypeMWS,
			},
			{
				Listen:        MSS_SERVER,
				ListenType:    constant.RelayTypeMWS,
				TCPRemotes:    []string{ECHO_SERVER},
				TransportType: constant.RelayTypeRaw,
			},
		},
	}

	var servers []*relay.Relay
	for _, c := range cfg.RelayConfigs {
		r, err := relay.NewRelay(c, cmgr.NewCmgr(cmgr.DummyConfig))
		if err != nil {
			zap.S().Fatal(err)
		}
		go r.ListenAndServe()
		servers = append(servers, r)
	}

	// Wait for init
	time.Sleep(time.Second)
	return servers
}

func TestRelay(t *testing.T) {
	testCases := []struct {
		name     string
		address  string
		protocol string
	}{
		{"Raw", RAW_LISTEN, "raw"},
		{"WS", WS_LISTEN, "ws"},
		{"WSS", WSS_LISTEN, "wss"},
		{"MWSS", MWSS_LISTEN, "mwss"},
		{"MTCP", MTCP_LISTEN, "mtcp"},
		{"MWS", MWS_LISTEN, "mws"},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			testRelayCommon(t, tc.address, tc.protocol, false)
		})
	}
}

func TestRelayConcurrent(t *testing.T) {
	testCases := []struct {
		name        string
		address     string
		concurrency int
	}{
		{"MWSS", MWSS_LISTEN, 10},
		{"MTCP", MTCP_LISTEN, 10},
		{"MWS", MWS_LISTEN, 10},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			testRelayCommon(t, tc.address, tc.name, true, tc.concurrency)
		})
	}
}

func testRelayCommon(t *testing.T, address, protocol string, concurrent bool, concurrency ...int) {
	t.Helper()
	msg := []byte("hello")

	runTest := func() error {
		res := echo.SendTcpMsg(msg, address)
		if !bytes.Equal(msg, res) {
			return fmt.Errorf("response mismatch: got %s, want %s", res, msg)
		}
		return nil
	}

	if concurrent {
		n := 10
		if len(concurrency) > 0 {
			n = concurrency[0]
		}
		g, ctx := errgroup.WithContext(context.Background())
		for i := 0; i < n; i++ {
			g.Go(func() error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
					return runTest()
				}
			})
		}
		require.NoError(t, g.Wait(), "Concurrent test failed")
	} else {
		require.NoError(t, runTest(), "Single test failed")
	}

	t.Logf("Test TCP over %s done!", protocol)
}

func TestRelayWithMaxConnectionCount(t *testing.T) {
	msg := []byte("hello")

	// First connection will be accepted
	go func() {
		err := echo.EchoTcpMsgLong(msg, time.Second, RAW_LISTEN_WITH_MAX_CONNECTION)
		require.NoError(t, err, "First connection should be accepted")
	}()

	// Wait for first connection
	time.Sleep(time.Second)

	// Second connection should be rejected
	err := echo.EchoTcpMsgLong(msg, time.Second, RAW_LISTEN_WITH_MAX_CONNECTION)
	require.Error(t, err, "Second connection should be rejected")
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
