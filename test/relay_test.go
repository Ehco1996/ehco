package test

import (
	"bytes"
	"context"
	"fmt"
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

	RAW_LISTEN = "0.0.0.0:1234"

	WS_LISTEN = "0.0.0.0:1235"
	WS_REMOTE = "ws://0.0.0.0:2000"
	WS_SERVER = "0.0.0.0:2000"

	WSS_LISTEN = "0.0.0.0:1236"
	WSS_REMOTE = "wss://0.0.0.0:2001"
	WSS_SERVER = "0.0.0.0:2001"
)

func TestMain(m *testing.M) {
	// Setup

	// change the idle timeout to 1 second to make connection close faster in test
	constant.IdleTimeOut = time.Second
	constant.ReadTimeOut = time.Second

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
		RelayConfigs: []*conf.Config{
			// raw
			{
				Listen:        RAW_LISTEN,
				ListenType:    constant.RelayTypeRaw,
				TCPRemotes:    []string{ECHO_SERVER},
				TransportType: constant.RelayTypeRaw,
				Options: &conf.Options{
					EnableUDP: true,
				},
			},
			// ws
			{
				Listen:        WS_LISTEN,
				ListenType:    constant.RelayTypeRaw,
				TCPRemotes:    []string{WS_REMOTE},
				TransportType: constant.RelayTypeWS,
				Options: &conf.Options{
					EnableUDP: true,
				},
			},
			{
				Listen:        WS_SERVER,
				ListenType:    constant.RelayTypeWS,
				TCPRemotes:    []string{ECHO_SERVER},
				TransportType: constant.RelayTypeRaw,
				Options: &conf.Options{
					EnableUDP: true,
				},
			},

			// wss
			{
				Listen:        WSS_LISTEN,
				ListenType:    constant.RelayTypeRaw,
				TCPRemotes:    []string{WSS_REMOTE},
				TransportType: constant.RelayTypeWSS,
				Options: &conf.Options{
					EnableUDP: true,
				},
			},
			{
				Listen:        WSS_SERVER,
				ListenType:    constant.RelayTypeWSS,
				TCPRemotes:    []string{ECHO_SERVER},
				TransportType: constant.RelayTypeRaw,
				Options: &conf.Options{
					EnableUDP: true,
				},
			},
		},
	}

	var servers []*relay.Relay
	for _, c := range cfg.RelayConfigs {
		c.Adjust()
		r, err := relay.NewRelay(c, cmgr.NewCmgr(cmgr.DummyConfig))
		if err != nil {
			zap.S().Fatal(err)
		}
		go r.ListenAndServe(context.TODO())
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
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			testTCPRelay(t, tc.address, tc.protocol, false)
			testUDPRelay(t, tc.address, false)
		})
	}
}

func TestRelayConcurrent(t *testing.T) {
	testCases := []struct {
		name        string
		address     string
		concurrency int
	}{
		{"Raw", RAW_LISTEN, 10},
		{"WS", WS_LISTEN, 10},
		{"WSS", WSS_LISTEN, 10},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			testTCPRelay(t, tc.address, tc.name, true, tc.concurrency)
			testUDPRelay(t, tc.address, true, tc.concurrency)
		})
	}
}

func testTCPRelay(t *testing.T, address, protocol string, concurrent bool, concurrency ...int) {
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

func testUDPRelay(t *testing.T, address string, concurrent bool, concurrency ...int) {
	t.Helper()
	msg := []byte("hello udp")

	runTest := func() error {
		res := echo.SendUdpMsg(msg, address)
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
	t.Logf("Test UDP over %s done!", address)
}

func TestRelayIdleTimeout(t *testing.T) {
	err := echo.EchoTcpMsgLong([]byte("hello"), time.Second, RAW_LISTEN)
	require.Error(t, err, "Connection should be rejected")
}
