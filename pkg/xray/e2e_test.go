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

func TestE2E_Trojan(t *testing.T) {
	runProtoE2E(t, e2eParams{
		proto:    ProtocolTrojan,
		tag:      XrayTrojanProxyTag,
		password: "trojan_test_password",
	})
}

func TestE2E_Vless(t *testing.T) {
	runProtoE2E(t, e2eParams{
		proto:    ProtocolVless,
		tag:      XrayVlessProxyTag,
		password: "11111111-1111-1111-1111-111111111111",
	})
}

func TestE2E_SS2022(t *testing.T) {
	runProtoE2E(t, e2eParams{
		proto:    ProtocolSS,
		tag:      XraySSProxyTag,
		method:   "2022-blake3-aes-128-gcm",
		password: ssUserKey,
	})
}

type e2eParams struct {
	proto, tag, method, password, flow string
}

func runProtoE2E(t *testing.T, p e2eParams) {
	t.Helper()

	backendAddr, stopBackend := startEcho(t)
	defer stopBackend()

	inboundPort := freePort(t)
	socksPort := freePort(t)

	user := &User{
		ID:       1,
		Protocol: p.proto,
		Password: p.password,
		Method:   p.method,
		Flow:     p.flow,
		Enable:   true,
	}
	syncSrv, syncURL := startSyncServer(t, []*User{user})
	defer syncSrv.Close()

	serverCfg := buildServerConfig(t, p, inboundPort, syncURL)

	xs := NewXrayServer(serverCfg)
	if err := xs.Setup(); err != nil {
		t.Fatalf("xs.Setup: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := xs.Start(ctx); err != nil {
		t.Fatalf("xs.Start: %v", err)
	}
	t.Cleanup(xs.Stop)

	// Wait for the initial sync to add the user via in-process inbound.Manager.
	waitFor(t, 5*time.Second, "user registered on inbound", func() bool {
		u, ok := xs.up.GetUser(1)
		return ok && u.running
	})

	clientInst, err := buildClientInstance(p, inboundPort, socksPort)
	if err != nil {
		t.Fatalf("client build: %v", err)
	}
	if err := clientInst.Start(); err != nil {
		t.Fatalf("client start: %v", err)
	}
	t.Cleanup(func() { _ = clientInst.Close() })

	dialer, err := proxy.SOCKS5("tcp", fmt.Sprintf("127.0.0.1:%d", socksPort), nil, proxy.Direct)
	if err != nil {
		t.Fatalf("socks5 dialer: %v", err)
	}

	// Both xray instances need a moment after Start before they accept conns.
	waitFor(t, 5*time.Second, "client xray ready", func() bool {
		c, err := dialer.Dial("tcp", backendAddr)
		if err != nil {
			return false
		}
		_ = c.Close()
		return true
	})

	// Round-trip data through the full chain: socks5 -> client xray -> protocol -> server xray -> metered outbound -> echo backend.
	conn, err := dialer.Dial("tcp", backendAddr)
	if err != nil {
		t.Fatalf("dial backend via socks: %v", err)
	}
	defer conn.Close()

	msg := []byte("hello e2e " + p.proto)
	if _, err := conn.Write(msg); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := make([]byte, len(msg))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != string(msg) {
		t.Fatalf("echo mismatch: got %q want %q", got, msg)
	}

	// Tracker should hold the live conn while it's open.
	if n := len(xs.tracker.List(1)); n < 1 {
		t.Fatalf("tracker: expected >=1 conn for user 1, got %d", n)
	}

	// Per-user byte counters should have advanced in both directions.
	waitFor(t, 2*time.Second, "user counters advanced", func() bool {
		u, ok := xs.up.GetUser(1)
		if !ok {
			return false
		}
		return atomic.LoadInt64(&u.UploadTraffic) > 0 && atomic.LoadInt64(&u.DownloadTraffic) > 0
	})

	// Kill the user's conn(s); the live conn should EOF promptly.
	killed := xs.tracker.KillByUser(1)
	if killed == 0 {
		t.Fatalf("KillByUser returned 0; expected >=1")
	}
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 1)
	if _, err := conn.Read(buf); err == nil {
		t.Fatalf("expected error reading from killed conn, got nil")
	}
}

// startEcho spins up a TCP echo server on a random port and returns its addr
// plus a cancel func.
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

// startSyncServer simulates the upstream that ehco's UserPool talks to:
// GET returns the user list; POST traffic uploads are accepted and discarded.
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

// freePort grabs a free TCP port and returns it. Race-prone but acceptable in tests.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
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

func buildServerConfig(t *testing.T, p e2eParams, port int, syncURL string) *config.Config {
	t.Helper()
	var inboundJSON string
	switch p.proto {
	case ProtocolTrojan:
		inboundJSON = fmt.Sprintf(`{
			"listen": "127.0.0.1",
			"port": %d,
			"protocol": "trojan",
			"tag": %q,
			"settings": {"clients": [{"password": %q, "email": "1"}], "network": "tcp,udp"},
			"streamSettings": {
				"network": "tcp",
				"security": "tls",
				"tlsSettings": {}
			}
		}`, port, p.tag, p.password)
	case ProtocolVless:
		inboundJSON = fmt.Sprintf(`{
			"listen": "127.0.0.1",
			"port": %d,
			"protocol": "vless",
			"tag": %q,
			"settings": {"clients": [{"id": %q, "email": "1"}], "decryption": "none"},
			"streamSettings": {
				"network": "tcp",
				"security": "tls",
				"tlsSettings": {}
			}
		}`, port, p.tag, p.password)
	case ProtocolSS:
		inboundJSON = fmt.Sprintf(`{
			"listen": "127.0.0.1",
			"port": %d,
			"protocol": "shadowsocks",
			"tag": %q,
			"settings": {
				"method": %q,
				"password": %q,
				"clients": [{"password": %q, "email": "1"}],
				"network": "tcp,udp"
			}
		}`, port, p.tag, p.method, ssServerKey, p.password)
	default:
		t.Fatalf("unknown proto %s", p.proto)
	}

	xrayJSON := fmt.Sprintf(`{
		"log": {"loglevel": "warning"},
		"inbounds": [%s]
	}`, inboundJSON)

	xc := &xConf.Config{}
	if err := json.Unmarshal([]byte(xrayJSON), xc); err != nil {
		t.Fatalf("parse xray cfg: %v", err)
	}

	return &config.Config{
		XRayConfig:          xc,
		SyncTrafficEndPoint: syncURL,
		ReloadInterval:      0,
	}
}

func TestE2E_TrojanUDP(t *testing.T) {
	runProtoUDPE2E(t, e2eParams{
		proto:    ProtocolTrojan,
		tag:      XrayTrojanProxyTag,
		password: "trojan_test_password",
	})
}

func TestE2E_SS2022UDP(t *testing.T) {
	runProtoUDPE2E(t, e2eParams{
		proto:    ProtocolSS,
		tag:      XraySSProxyTag,
		method:   "2022-blake3-aes-128-gcm",
		password: ssUserKey,
	})
}

// runProtoUDPE2E mirrors runProtoE2E but the client carries traffic over UDP.
// Instead of socks5 (whose UDP ASSOCIATE flow x/net/proxy doesn't implement),
// the client xray exposes a dokodemo-door UDP inbound that pre-routes every
// packet to the backend addr — so the test client can be a plain net.Dial("udp").
func runProtoUDPE2E(t *testing.T, p e2eParams) {
	t.Helper()

	backendAddr, stopBackend := startUDPEcho(t)
	defer stopBackend()
	backendIP, backendPortStr, err := net.SplitHostPort(backendAddr)
	if err != nil {
		t.Fatalf("split backend addr: %v", err)
	}
	backendPort, err := strconv.Atoi(backendPortStr)
	if err != nil {
		t.Fatalf("parse backend port: %v", err)
	}

	inboundPort := freePort(t)
	clientUDPPort := freeUDPPort(t)

	user := &User{
		ID:       1,
		Protocol: p.proto,
		Password: p.password,
		Method:   p.method,
		Flow:     p.flow,
		Enable:   true,
	}
	syncSrv, syncURL := startSyncServer(t, []*User{user})
	defer syncSrv.Close()

	serverCfg := buildServerConfig(t, p, inboundPort, syncURL)
	xs := NewXrayServer(serverCfg)
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

	clientInst, err := buildClientUDPInstance(p, inboundPort, clientUDPPort, backendIP, backendPort)
	if err != nil {
		t.Fatalf("client build: %v", err)
	}
	if err := clientInst.Start(); err != nil {
		t.Fatalf("client start: %v", err)
	}
	t.Cleanup(func() { _ = clientInst.Close() })

	clientAddr := fmt.Sprintf("127.0.0.1:%d", clientUDPPort)
	msg := []byte("hello udp " + p.proto)

	// UDP is best-effort and the client xray needs a moment after Start to
	// open its protocol session. Retry the send/recv cycle until echo lands.
	got, err := udpRoundtripWithRetry(clientAddr, msg, 8*time.Second)
	if err != nil {
		t.Fatalf("udp roundtrip: %v", err)
	}
	if !bytes.Equal(got, msg) {
		t.Fatalf("echo mismatch: got %q want %q", got, msg)
	}

	// Tracker should hold the live UDP session while the dialed conn is open
	// (xray's UDP idle timeout is generous; the assertion is racy only in
	// pathological CI timing, and the metered outbound's defer Unregister
	// only fires after the UDP session times out or is killed).
	waitFor(t, 2*time.Second, "udp conn registered in tracker", func() bool {
		for _, c := range xs.tracker.List(1) {
			if c.Network == "udp" {
				return true
			}
		}
		return false
	})

	waitFor(t, 2*time.Second, "user counters advanced", func() bool {
		u, ok := xs.up.GetUser(1)
		if !ok {
			return false
		}
		return atomic.LoadInt64(&u.UploadTraffic) > 0 && atomic.LoadInt64(&u.DownloadTraffic) > 0
	})

	killed := xs.tracker.KillByUser(1)
	if killed == 0 {
		t.Fatalf("KillByUser returned 0; expected >=1")
	}
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

func freeUDPPort(t *testing.T) int {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freeUDPPort: %v", err)
	}
	defer pc.Close()
	return pc.LocalAddr().(*net.UDPAddr).Port
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

// buildClientUDPInstance builds a client xray with a dokodemo-door UDP inbound
// on clientUDPPort that forwards every packet to backendIP:backendPort, going
// out through the protocol outbound dialing 127.0.0.1:inboundPort.
func buildClientUDPInstance(p e2eParams, inboundPort, clientUDPPort int, backendIP string, backendPort int) (*core.Instance, error) {
	var outboundJSON string
	switch p.proto {
	case ProtocolTrojan:
		outboundJSON = fmt.Sprintf(`{
			"protocol": "trojan",
			"settings": {"servers": [{"address": "127.0.0.1", "port": %d, "password": %q}]},
			"streamSettings": {"network": "tcp", "security": "tls", "tlsSettings": {"allowInsecure": true}}
		}`, inboundPort, p.password)
	case ProtocolSS:
		clientKey := ssServerKey + ":" + ssUserKey
		outboundJSON = fmt.Sprintf(`{
			"protocol": "shadowsocks",
			"settings": {"servers": [{"address": "127.0.0.1", "port": %d, "method": %q, "password": %q}]}
		}`, inboundPort, p.method, clientKey)
	default:
		return nil, fmt.Errorf("UDP not supported for proto %s", p.proto)
	}

	clientJSON := fmt.Sprintf(`{
		"log": {"loglevel": "warning"},
		"inbounds": [{
			"listen": "127.0.0.1",
			"port": %d,
			"protocol": "dokodemo-door",
			"settings": {
				"address": %q,
				"port": %d,
				"network": "udp"
			}
		}],
		"outbounds": [%s]
	}`, clientUDPPort, backendIP, backendPort, outboundJSON)

	cc := &xConf.Config{}
	if err := json.Unmarshal([]byte(clientJSON), cc); err != nil {
		return nil, fmt.Errorf("parse client cfg: %w", err)
	}
	core_, err := cc.Build()
	if err != nil {
		return nil, fmt.Errorf("build core cfg: %w", err)
	}
	return core.New(core_)
}

// buildClientInstance constructs a plain xray-core client: socks5 inbound on
// socksPort + protocol outbound dialing 127.0.0.1:inboundPort.
func buildClientInstance(p e2eParams, inboundPort, socksPort int) (*core.Instance, error) {
	var outboundJSON string
	switch p.proto {
	case ProtocolTrojan:
		outboundJSON = fmt.Sprintf(`{
			"protocol": "trojan",
			"settings": {"servers": [{"address": "127.0.0.1", "port": %d, "password": %q}]},
			"streamSettings": {"network": "tcp", "security": "tls", "tlsSettings": {"allowInsecure": true}}
		}`, inboundPort, p.password)
	case ProtocolVless:
		outboundJSON = fmt.Sprintf(`{
			"protocol": "vless",
			"settings": {"vnext": [{"address": "127.0.0.1", "port": %d, "users": [{"id": %q, "encryption": "none"}]}]},
			"streamSettings": {"network": "tcp", "security": "tls", "tlsSettings": {"allowInsecure": true}}
		}`, inboundPort, p.password)
	case ProtocolSS:
		// Multi-user 2022 client password format is "<server_key>:<user_key>".
		clientKey := ssServerKey + ":" + ssUserKey
		outboundJSON = fmt.Sprintf(`{
			"protocol": "shadowsocks",
			"settings": {"servers": [{"address": "127.0.0.1", "port": %d, "method": %q, "password": %q}]}
		}`, inboundPort, p.method, clientKey)
	default:
		return nil, fmt.Errorf("unknown proto %s", p.proto)
	}

	clientJSON := fmt.Sprintf(`{
		"log": {"loglevel": "warning"},
		"inbounds": [{
			"listen": "127.0.0.1",
			"port": %d,
			"protocol": "socks",
			"settings": {"auth": "noauth", "udp": false}
		}],
		"outbounds": [%s]
	}`, socksPort, outboundJSON)

	cc := &xConf.Config{}
	if err := json.Unmarshal([]byte(clientJSON), cc); err != nil {
		return nil, fmt.Errorf("parse client cfg: %w", err)
	}
	core_, err := cc.Build()
	if err != nil {
		return nil, fmt.Errorf("build core cfg: %w", err)
	}
	return core.New(core_)
}

func TestE2E_VlessReality(t *testing.T) {
	const (
		serverName = "www.example.com"
		shortID    = "0123456789abcdef"
		userUUID   = "11111111-1111-1111-1111-111111111111"
	)

	backendAddr, stopBackend := startEcho(t)
	defer stopBackend()

	destAddr, stopDest := startTLSDest(t)
	defer stopDest()

	inboundPort := freePort(t)
	socksPort := freePort(t)

	privB64, pubB64 := genRealityKeyPair(t)

	user := &User{
		ID:       1,
		Protocol: ProtocolVless,
		Password: userUUID,
		Enable:   true,
	}
	syncSrv, syncURL := startSyncServer(t, []*User{user})
	defer syncSrv.Close()

	serverCfg := buildVlessRealityServerConfig(t, inboundPort, syncURL,
		privB64, shortID, serverName, destAddr, userUUID)

	xs := NewXrayServer(serverCfg)
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

	clientInst, err := buildVlessRealityClientInstance(inboundPort, socksPort,
		pubB64, shortID, serverName, userUUID)
	if err != nil {
		t.Fatalf("client build: %v", err)
	}
	if err := clientInst.Start(); err != nil {
		t.Fatalf("client start: %v", err)
	}
	t.Cleanup(func() { _ = clientInst.Close() })

	dialer, err := proxy.SOCKS5("tcp", fmt.Sprintf("127.0.0.1:%d", socksPort), nil, proxy.Direct)
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
		t.Fatalf("dial backend: %v", err)
	}
	defer conn.Close()

	msg := []byte("hello reality")
	if _, err := conn.Write(msg); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := make([]byte, len(msg))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, msg) {
		t.Fatalf("echo mismatch: got %q want %q", got, msg)
	}

	if n := len(xs.tracker.List(1)); n < 1 {
		t.Fatalf("tracker: expected >=1 conn, got %d", n)
	}
	waitFor(t, 2*time.Second, "user counters advanced", func() bool {
		u, ok := xs.up.GetUser(1)
		return ok && atomic.LoadInt64(&u.UploadTraffic) > 0 && atomic.LoadInt64(&u.DownloadTraffic) > 0
	})

	killed := xs.tracker.KillByUser(1)
	if killed == 0 {
		t.Fatalf("KillByUser returned 0; expected >=1")
	}
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := conn.Read(make([]byte, 1)); err == nil {
		t.Fatalf("expected error reading from killed conn, got nil")
	}
}

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

// startTLSDest spins up a TLS server that accepts connections and discards
// data — REALITY uses it as cover only; authenticated clients never reach
// the dest's data path.
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

func buildVlessRealityServerConfig(t *testing.T, port int, syncURL, privKey, shortID, serverName, destAddr, userUUID string) *config.Config {
	t.Helper()
	xrayJSON := fmt.Sprintf(`{
		"log": {"loglevel": "warning"},
		"inbounds": [{
			"listen": "127.0.0.1",
			"port": %d,
			"protocol": "vless",
			"tag": %q,
			"settings": {
				"clients": [{"id": %q, "email": "1"}],
				"decryption": "none"
			},
			"streamSettings": {
				"network": "tcp",
				"security": "reality",
				"realitySettings": {
					"show": false,
					"dest": %q,
					"xver": 0,
					"serverNames": [%q],
					"privateKey": %q,
					"shortIds": [%q]
				}
			}
		}]
	}`, port, XrayVlessProxyTag, userUUID, destAddr, serverName, privKey, shortID)

	xc := &xConf.Config{}
	if err := json.Unmarshal([]byte(xrayJSON), xc); err != nil {
		t.Fatalf("parse xray cfg: %v", err)
	}
	return &config.Config{
		XRayConfig:          xc,
		SyncTrafficEndPoint: syncURL,
	}
}

func buildVlessRealityClientInstance(inboundPort, socksPort int, pubKey, shortID, serverName, userUUID string) (*core.Instance, error) {
	clientJSON := fmt.Sprintf(`{
		"log": {"loglevel": "warning"},
		"inbounds": [{
			"listen": "127.0.0.1",
			"port": %d,
			"protocol": "socks",
			"settings": {"auth": "noauth", "udp": false}
		}],
		"outbounds": [{
			"protocol": "vless",
			"settings": {
				"vnext": [{
					"address": "127.0.0.1",
					"port": %d,
					"users": [{"id": %q, "encryption": "none"}]
				}]
			},
			"streamSettings": {
				"network": "tcp",
				"security": "reality",
				"realitySettings": {
					"serverName": %q,
					"fingerprint": "chrome",
					"publicKey": %q,
					"shortId": %q
				}
			}
		}]
	}`, socksPort, inboundPort, userUUID, serverName, pubKey, shortID)

	cc := &xConf.Config{}
	if err := json.Unmarshal([]byte(clientJSON), cc); err != nil {
		return nil, fmt.Errorf("parse client cfg: %w", err)
	}
	core_, err := cc.Build()
	if err != nil {
		return nil, fmt.Errorf("build core cfg: %w", err)
	}
	return core.New(core_)
}
