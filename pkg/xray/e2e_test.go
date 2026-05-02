package xray

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ehco1996/ehco/internal/config"
	ehcoTls "github.com/Ehco1996/ehco/internal/tls"
	"github.com/xtls/xray-core/core"
	xConf "github.com/xtls/xray-core/infra/conf"
	"golang.org/x/net/proxy"
)

// SS2022 PSKs used by both server and client. Real deployments would generate
// fresh keys; these are fixed only so the test is deterministic.
const (
	ssServerKey = "AAAAAAAAAAAAAAAAAAAAAA==" // 16 bytes of zero
	ssUserKey   = "MTIzNDU2Nzg5MDEyMzQ1Ng==" // base64("1234567890123456")
)

// scenario captures the per-test variation; runScenario drives the shared
// orchestration (sync HTTP, server xray, client xray, assertions).
type scenario struct {
	proto, tag, method, password, flow string

	// startBackend opens an echo server (TCP or UDP) and returns its addr.
	startBackend func(t *testing.T) (addr string, stop func())

	// serverCfg builds the *config.Config the XrayServer will be started from.
	serverCfg func(t *testing.T, inboundPort int, syncURL string) *config.Config

	// clientFactory builds a plain xray client instance and returns the
	// caller-facing dial addr (the socks5 listen for TCP, the dokodemo-door
	// listen for UDP).
	clientFactory func(t *testing.T, inboundPort int, backendAddr string) (inst *core.Instance, dialAddr string)

	// runSession does the data-path roundtrip + tracker/counter/kill checks.
	runSession func(t *testing.T, xs *XrayServer, backendAddr, dialAddr, msg string)
}

func TestE2E_Trojan(t *testing.T) {
	p := e2eParams{ProtocolTrojan, XrayTrojanProxyTag, "", "trojan_test_password", ""}
	runScenario(t, scenario{
		proto: p.proto, tag: p.tag, method: p.method, password: p.password, flow: p.flow,
		startBackend:  startEcho,
		serverCfg:     plainServerCfgFn(p),
		clientFactory: socksClientFactory(p),
		runSession:    tcpSession,
	})
}

func TestE2E_Vless(t *testing.T) {
	p := e2eParams{ProtocolVless, XrayVlessProxyTag, "", "11111111-1111-1111-1111-111111111111", ""}
	runScenario(t, scenario{
		proto: p.proto, tag: p.tag, method: p.method, password: p.password, flow: p.flow,
		startBackend:  startEcho,
		serverCfg:     plainServerCfgFn(p),
		clientFactory: socksClientFactory(p),
		runSession:    tcpSession,
	})
}

func TestE2E_SS2022(t *testing.T) {
	p := e2eParams{ProtocolSS, XraySSProxyTag, "2022-blake3-aes-128-gcm", ssUserKey, ""}
	runScenario(t, scenario{
		proto: p.proto, tag: p.tag, method: p.method, password: p.password, flow: p.flow,
		startBackend:  startEcho,
		serverCfg:     plainServerCfgFn(p),
		clientFactory: socksClientFactory(p),
		runSession:    tcpSession,
	})
}

func TestE2E_TrojanUDP(t *testing.T) {
	p := e2eParams{ProtocolTrojan, XrayTrojanProxyTag, "", "trojan_test_password", ""}
	runScenario(t, scenario{
		proto: p.proto, tag: p.tag, method: p.method, password: p.password, flow: p.flow,
		startBackend:  startUDPEcho,
		serverCfg:     plainServerCfgFn(p),
		clientFactory: dokodemoUDPClientFactory(p),
		runSession:    udpSession,
	})
}

func TestE2E_SS2022UDP(t *testing.T) {
	p := e2eParams{ProtocolSS, XraySSProxyTag, "2022-blake3-aes-128-gcm", ssUserKey, ""}
	runScenario(t, scenario{
		proto: p.proto, tag: p.tag, method: p.method, password: p.password, flow: p.flow,
		startBackend:  startUDPEcho,
		serverCfg:     plainServerCfgFn(p),
		clientFactory: dokodemoUDPClientFactory(p),
		runSession:    udpSession,
	})
}

func TestE2E_VlessReality(t *testing.T) {
	const (
		serverName = "www.example.com"
		shortID    = "0123456789abcdef"
		userUUID   = "11111111-1111-1111-1111-111111111111"
	)
	destAddr, stopDest := startTLSDest(t)
	t.Cleanup(stopDest)
	privB64, pubB64 := genRealityKeyPair(t)

	p := e2eParams{ProtocolVless, XrayVlessProxyTag, "", userUUID, ""}
	runScenario(t, scenario{
		proto: p.proto, tag: p.tag, method: p.method, password: p.password, flow: p.flow,
		startBackend: startEcho,
		serverCfg: func(t *testing.T, port int, syncURL string) *config.Config {
			return realityServerCfg(t, port, syncURL, privB64, shortID, serverName, destAddr, userUUID)
		},
		clientFactory: realityClientFactory(pubB64, shortID, serverName, userUUID),
		runSession:    tcpSession,
	})
}

type e2eParams struct {
	proto, tag, method, password, flow string
}

func runScenario(t *testing.T, s scenario) {
	t.Helper()

	backendAddr, stopBackend := s.startBackend(t)
	defer stopBackend()

	inboundPort := freePort(t)

	user := &User{
		ID: 1, Protocol: s.proto, Password: s.password,
		Method: s.method, Flow: s.flow, Enable: true,
	}
	syncSrv, syncURL := startSyncServer(t, []*User{user})
	defer syncSrv.Close()

	xs := NewXrayServer(s.serverCfg(t, inboundPort, syncURL))
	if err := xs.Setup(); err != nil {
		t.Fatalf("xs.Setup: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := xs.Start(ctx); err != nil {
		t.Fatalf("xs.Start: %v", err)
	}
	t.Cleanup(xs.Stop)

	waitFor(t, 5*time.Second, "user registered on inbound", func() bool {
		u, ok := xs.up.GetUser(1)
		return ok && u.running
	})

	inst, dialAddr := s.clientFactory(t, inboundPort, backendAddr)
	if err := inst.Start(); err != nil {
		t.Fatalf("client start: %v", err)
	}
	t.Cleanup(func() { _ = inst.Close() })

	s.runSession(t, xs, backendAddr, dialAddr, "hello e2e "+s.proto)
}

// tcpSession runs the socks5 → backend echo roundtrip, then asserts the
// tracker/counters/kill behaviour for the live conn.
func tcpSession(t *testing.T, xs *XrayServer, backendAddr, socksAddr, msg string) {
	t.Helper()
	dialer, err := proxy.SOCKS5("tcp", socksAddr, nil, proxy.Direct)
	if err != nil {
		t.Fatalf("socks5 dialer: %v", err)
	}
	waitFor(t, 8*time.Second, "client xray ready", func() bool {
		c, err := dialer.Dial("tcp", backendAddr)
		if err != nil {
			return false
		}
		_ = c.Close()
		return true
	})

	conn, err := dialer.Dial("tcp", backendAddr)
	if err != nil {
		t.Fatalf("dial backend via socks: %v", err)
	}
	defer conn.Close()

	payload := []byte(msg)
	if _, err := conn.Write(payload); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("echo mismatch: got %q want %q", got, payload)
	}

	if n := len(xs.tracker.List(1)); n < 1 {
		t.Fatalf("tracker: expected >=1 conn, got %d", n)
	}
	assertCountersAdvanced(t, xs)

	if killed := xs.tracker.KillByUser(1); killed == 0 {
		t.Fatalf("KillByUser returned 0; expected >=1")
	}
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := conn.Read(make([]byte, 1)); err == nil {
		t.Fatalf("expected error reading from killed conn, got nil")
	}
}

// udpSession runs the dokodemo-door → backend UDP-echo roundtrip and asserts
// the same tracker/counter/kill behaviour as tcpSession (without the post-kill
// EOF assertion — UDP has no "connection closed" signal back to the client).
func udpSession(t *testing.T, xs *XrayServer, _ string, dialAddr, msg string) {
	t.Helper()
	payload := []byte(msg)
	got, err := udpRoundtripWithRetry(dialAddr, payload, 8*time.Second)
	if err != nil {
		t.Fatalf("udp roundtrip: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("echo mismatch: got %q want %q", got, payload)
	}

	waitFor(t, 2*time.Second, "udp conn registered in tracker", func() bool {
		for _, c := range xs.tracker.List(1) {
			if c.Network == "udp" {
				return true
			}
		}
		return false
	})
	assertCountersAdvanced(t, xs)

	if killed := xs.tracker.KillByUser(1); killed == 0 {
		t.Fatalf("KillByUser returned 0; expected >=1")
	}
}

func assertCountersAdvanced(t *testing.T, xs *XrayServer) {
	t.Helper()
	waitFor(t, 2*time.Second, "user counters advanced", func() bool {
		u, ok := xs.up.GetUser(1)
		return ok && atomic.LoadInt64(&u.UploadTraffic) > 0 && atomic.LoadInt64(&u.DownloadTraffic) > 0
	})
}

// startEcho spins up a TCP echo server on a random port.
func startEcho(t *testing.T) (string, func()) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("echo listen: %v", err)
	}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}(c)
		}
	}()
	return l.Addr().String(), func() { _ = l.Close() }
}

// startUDPEcho returns the addr of a UDP packet echo server.
func startUDPEcho(t *testing.T) (string, func()) {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("udp echo listen: %v", err)
	}
	go func() {
		buf := make([]byte, 64*1024)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = pc.WriteTo(buf[:n], addr)
		}
	}()
	return pc.LocalAddr().String(), func() { _ = pc.Close() }
}

// startSyncServer simulates ehco's upstream: GET returns the user list; POST
// traffic uploads are accepted and discarded.
func startSyncServer(t *testing.T, users []*User) (*httptest.Server, string) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(SyncUserConfigsResp{Users: users})
		case http.MethodPost:
			_, _ = io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	srv := httptest.NewServer(mux)
	return srv, srv.URL + "/"
}

// freePort grabs a free TCP port (also used as UDP listen — race-prone but OK in tests).
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func freeUDPPort(t *testing.T) int {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freeUDPPort: %v", err)
	}
	defer pc.Close()
	return pc.LocalAddr().(*net.UDPAddr).Port
}

func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for: %s", what)
}

func udpRoundtripWithRetry(addr string, msg []byte, total time.Duration) ([]byte, error) {
	deadline := time.Now().Add(total)
	var lastErr error
	for time.Now().Before(deadline) {
		c, err := net.Dial("udp", addr)
		if err != nil {
			lastErr = err
			time.Sleep(100 * time.Millisecond)
			continue
		}
		_ = c.SetDeadline(time.Now().Add(500 * time.Millisecond))
		if _, err := c.Write(msg); err != nil {
			_ = c.Close()
			lastErr = err
			time.Sleep(100 * time.Millisecond)
			continue
		}
		buf := make([]byte, len(msg)*2)
		n, err := c.Read(buf)
		_ = c.Close()
		if err == nil {
			return buf[:n], nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("udp roundtrip failed within %s: %w", total, lastErr)
}

// --- server config builders ----------------------------------------------

// plainServerCfgFn returns a serverCfg closure that builds a non-REALITY
// xray server inbound for the given protocol.
func plainServerCfgFn(p e2eParams) func(*testing.T, int, string) *config.Config {
	return func(t *testing.T, port int, syncURL string) *config.Config {
		return parseConfig(t, syncURL, plainInboundJSON(t, p, port))
	}
}

func plainInboundJSON(t *testing.T, p e2eParams, port int) string {
	t.Helper()
	switch p.proto {
	case ProtocolTrojan:
		return fmt.Sprintf(`{
			"listen": "127.0.0.1", "port": %d, "protocol": "trojan", "tag": %q,
			"settings": {"clients": [{"password": %q, "email": "1"}], "network": "tcp,udp"},
			"streamSettings": {"network": "tcp", "security": "tls", "tlsSettings": {}}
		}`, port, p.tag, p.password)
	case ProtocolVless:
		return fmt.Sprintf(`{
			"listen": "127.0.0.1", "port": %d, "protocol": "vless", "tag": %q,
			"settings": {"clients": [{"id": %q, "email": "1"}], "decryption": "none"},
			"streamSettings": {"network": "tcp", "security": "tls", "tlsSettings": {}}
		}`, port, p.tag, p.password)
	case ProtocolSS:
		return fmt.Sprintf(`{
			"listen": "127.0.0.1", "port": %d, "protocol": "shadowsocks", "tag": %q,
			"settings": {
				"method": %q, "password": %q,
				"clients": [{"password": %q, "email": "1"}],
				"network": "tcp,udp"
			}
		}`, port, p.tag, p.method, ssServerKey, p.password)
	}
	t.Fatalf("unknown proto %s", p.proto)
	return ""
}

func realityServerCfg(t *testing.T, port int, syncURL, privKey, shortID, serverName, destAddr, userUUID string) *config.Config {
	t.Helper()
	inbound := fmt.Sprintf(`{
		"listen": "127.0.0.1", "port": %d, "protocol": "vless", "tag": %q,
		"settings": {"clients": [{"id": %q, "email": "1"}], "decryption": "none"},
		"streamSettings": {
			"network": "tcp", "security": "reality",
			"realitySettings": {
				"show": false, "dest": %q, "xver": 0,
				"serverNames": [%q], "privateKey": %q, "shortIds": [%q]
			}
		}
	}`, port, XrayVlessProxyTag, userUUID, destAddr, serverName, privKey, shortID)
	return parseConfig(t, syncURL, inbound)
}

func parseConfig(t *testing.T, syncURL, inboundJSON string) *config.Config {
	t.Helper()
	xc := &xConf.Config{}
	xrayJSON := fmt.Sprintf(`{"log": {"loglevel": "warning"}, "inbounds": [%s]}`, inboundJSON)
	if err := json.Unmarshal([]byte(xrayJSON), xc); err != nil {
		t.Fatalf("parse xray cfg: %v", err)
	}
	return &config.Config{XRayConfig: xc, SyncTrafficEndPoint: syncURL}
}

// --- client xray factories -----------------------------------------------

func socksClientFactory(p e2eParams) func(*testing.T, int, string) (*core.Instance, string) {
	return func(t *testing.T, inboundPort int, _ string) (*core.Instance, string) {
		t.Helper()
		socksPort := freePort(t)
		clientJSON := fmt.Sprintf(`{
			"log": {"loglevel": "warning"},
			"inbounds": [{
				"listen": "127.0.0.1", "port": %d, "protocol": "socks",
				"settings": {"auth": "noauth", "udp": false}
			}],
			"outbounds": [%s]
		}`, socksPort, plainOutboundJSON(t, p, inboundPort))
		return mustBuildInstance(t, clientJSON), fmt.Sprintf("127.0.0.1:%d", socksPort)
	}
}

func dokodemoUDPClientFactory(p e2eParams) func(*testing.T, int, string) (*core.Instance, string) {
	return func(t *testing.T, inboundPort int, backendAddr string) (*core.Instance, string) {
		t.Helper()
		backendIP, backendPortStr, err := net.SplitHostPort(backendAddr)
		if err != nil {
			t.Fatalf("split backend: %v", err)
		}
		backendPort, err := strconv.Atoi(backendPortStr)
		if err != nil {
			t.Fatalf("parse backend port: %v", err)
		}
		clientUDPPort := freeUDPPort(t)
		clientJSON := fmt.Sprintf(`{
			"log": {"loglevel": "warning"},
			"inbounds": [{
				"listen": "127.0.0.1", "port": %d, "protocol": "dokodemo-door",
				"settings": {"address": %q, "port": %d, "network": "udp"}
			}],
			"outbounds": [%s]
		}`, clientUDPPort, backendIP, backendPort, plainOutboundJSON(t, p, inboundPort))
		return mustBuildInstance(t, clientJSON), fmt.Sprintf("127.0.0.1:%d", clientUDPPort)
	}
}

func realityClientFactory(pubKey, shortID, serverName, userUUID string) func(*testing.T, int, string) (*core.Instance, string) {
	return func(t *testing.T, inboundPort int, _ string) (*core.Instance, string) {
		t.Helper()
		socksPort := freePort(t)
		clientJSON := fmt.Sprintf(`{
			"log": {"loglevel": "warning"},
			"inbounds": [{
				"listen": "127.0.0.1", "port": %d, "protocol": "socks",
				"settings": {"auth": "noauth", "udp": false}
			}],
			"outbounds": [{
				"protocol": "vless",
				"settings": {"vnext": [{
					"address": "127.0.0.1", "port": %d,
					"users": [{"id": %q, "encryption": "none"}]
				}]},
				"streamSettings": {
					"network": "tcp", "security": "reality",
					"realitySettings": {
						"serverName": %q, "fingerprint": "chrome",
						"publicKey": %q, "shortId": %q
					}
				}
			}]
		}`, socksPort, inboundPort, userUUID, serverName, pubKey, shortID)
		return mustBuildInstance(t, clientJSON), fmt.Sprintf("127.0.0.1:%d", socksPort)
	}
}

func plainOutboundJSON(t *testing.T, p e2eParams, inboundPort int) string {
	t.Helper()
	switch p.proto {
	case ProtocolTrojan:
		return fmt.Sprintf(`{
			"protocol": "trojan",
			"settings": {"servers": [{"address": "127.0.0.1", "port": %d, "password": %q}]},
			"streamSettings": {"network": "tcp", "security": "tls", "tlsSettings": {"allowInsecure": true}}
		}`, inboundPort, p.password)
	case ProtocolVless:
		return fmt.Sprintf(`{
			"protocol": "vless",
			"settings": {"vnext": [{"address": "127.0.0.1", "port": %d, "users": [{"id": %q, "encryption": "none"}]}]},
			"streamSettings": {"network": "tcp", "security": "tls", "tlsSettings": {"allowInsecure": true}}
		}`, inboundPort, p.password)
	case ProtocolSS:
		// Multi-user 2022 client password format is "<server_key>:<user_key>".
		clientKey := ssServerKey + ":" + ssUserKey
		return fmt.Sprintf(`{
			"protocol": "shadowsocks",
			"settings": {"servers": [{"address": "127.0.0.1", "port": %d, "method": %q, "password": %q}]}
		}`, inboundPort, p.method, clientKey)
	}
	t.Fatalf("unknown proto %s", p.proto)
	return ""
}

func mustBuildInstance(t *testing.T, clientJSON string) *core.Instance {
	t.Helper()
	cc := &xConf.Config{}
	if err := json.Unmarshal([]byte(clientJSON), cc); err != nil {
		t.Fatalf("parse client cfg: %v", err)
	}
	core_, err := cc.Build()
	if err != nil {
		t.Fatalf("build core cfg: %v", err)
	}
	inst, err := core.New(core_)
	if err != nil {
		t.Fatalf("core.New: %v", err)
	}
	return inst
}

// --- REALITY-specific helpers --------------------------------------------

// genRealityKeyPair returns a base64.RawURLEncoding x25519 (private, public)
// pair, mirroring xray's `xray x25519` command.
func genRealityKeyPair(t *testing.T) (privB64, pubB64 string) {
	t.Helper()
	priv := make([]byte, 32)
	if _, err := rand.Read(priv); err != nil {
		t.Fatalf("rand: %v", err)
	}
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64
	key, err := ecdh.X25519().NewPrivateKey(priv)
	if err != nil {
		t.Fatalf("x25519 key: %v", err)
	}
	pub := key.PublicKey().Bytes()
	enc := base64.RawURLEncoding
	return enc.EncodeToString(priv), enc.EncodeToString(pub)
}

// startTLSDest spins up a TLS server that accepts conns and discards data —
// REALITY uses it as cover only; authenticated clients never reach the dest's
// data path.
func startTLSDest(t *testing.T) (string, func()) {
	t.Helper()
	if err := ehcoTls.InitTlsCfg(); err != nil {
		t.Fatalf("init tls: %v", err)
	}
	cert, err := tls.X509KeyPair(ehcoTls.DefaultTLSConfigCertBytes, ehcoTls.DefaultTLSConfigKeyBytes)
	if err != nil {
		t.Fatalf("x509 keypair: %v", err)
	}
	l, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatalf("tls dest listen: %v", err)
	}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = io.Copy(io.Discard, c)
			}(c)
		}
	}()
	return l.Addr().String(), func() { _ = l.Close() }
}
