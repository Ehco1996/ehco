package test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"
	"time"

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
		server.Stop()
	}

	os.Exit(code)
}

func startRelayServers() []*relay.Relay {
	options := conf.Options{
		EnableUDP:      true,
		IdleTimeoutSec: 1,
		ReadTimeoutSec: 1,
	}
	cfg := config.Config{
		RelayConfigs: []*conf.Config{
			// raw
			{
				Label:         "raw",
				Listen:        RAW_LISTEN,
				ListenType:    constant.RelayTypeRaw,
				Remotes:       []string{ECHO_SERVER},
				TransportType: constant.RelayTypeRaw,
				Options:       &options,
			},
			// ws
			{
				Label:         "ws-in",
				Listen:        WS_LISTEN,
				ListenType:    constant.RelayTypeRaw,
				Remotes:       []string{WS_REMOTE},
				TransportType: constant.RelayTypeWS,
				Options:       &options,
			},
			{
				Label:         "ws-out",
				Listen:        WS_SERVER,
				ListenType:    constant.RelayTypeWS,
				Remotes:       []string{ECHO_SERVER},
				TransportType: constant.RelayTypeRaw,
				Options:       &options,
			},

			// wss
			{
				Label:         "wss-in",
				Listen:        WSS_LISTEN,
				ListenType:    constant.RelayTypeRaw,
				Remotes:       []string{WSS_REMOTE},
				TransportType: constant.RelayTypeWSS,
				Options:       &options,
			},
			{
				Label:         "wss-out",
				Listen:        WSS_SERVER,
				ListenType:    constant.RelayTypeWSS,
				Remotes:       []string{ECHO_SERVER},
				TransportType: constant.RelayTypeRaw,
				Options:       &options,
			},
		},
	}
	cfg.Adjust()

	var servers []*relay.Relay
	for _, c := range cfg.RelayConfigs {
		c.Adjust()
		r, err := relay.NewRelay(c, nil)
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
	err := echo.EchoTcpMsgLong([]byte("hello"), time.Second*2, RAW_LISTEN)
	require.Error(t, err, "Connection should be rejected")
}
