package test

import (
	"github.com/Ehco1996/ehco/internal/relay"
	"testing"
	"time"
)

var echoHost = "0.0.0.0"
var echoPort = 9002

var rawLocal = "0.0.0.0:1234"
var rawRemote = "0.0.0.0:9002"

var wsListen = "0.0.0.0:1236"

var wsLocal = "0.0.0.0:1235"
var wsRemote = "wss://0.0.0.0:1236"

func init() {
	// Start the new echo server.
	go RunEchoServer(echoHost, echoPort)

	// init tls
	relay.InitTlsCfg()

	// Start the relay server
	go func() {
		r, err := relay.NewRelay(rawLocal, relay.Listen_RAW, rawRemote, relay.Transport_RAW)
		if err != nil {
			panic(err)
		}
		stop := make(chan error)
		stop <- r.ListenAndServe()
	}()

	// Start relay listen ws server
	go func() {
		r, err := relay.NewRelay(wsListen, relay.Listen_WSS, rawRemote, relay.Transport_RAW)
		if err != nil {
			panic(err)
		}
		stop := make(chan error)
		stop <- r.ListenAndServe()
	}()
	// Start relay over ws server
	go func() {
		r, err := relay.NewRelay(wsLocal, relay.Listen_RAW, wsRemote, relay.Transport_WSS)
		if err != nil {
			panic(err)
		}
		stop := make(chan error)
		stop <- r.ListenAndServe()
	}()
	// wait for  init
	time.Sleep(time.Second)
}

func TestRelay(t *testing.T) {

	msg := []byte("hello")
	// test tcp
	res := SendTcpMsg(msg, rawLocal)
	if string(res) != string(msg) {
		t.Fatal(res)
	}
	t.Log("test tcp down!")

	// test udp
	res = SendUdpMsg(msg, rawLocal)
	if string(res) != string(msg) {
		t.Fatal(res)
	}
	t.Log("test udp down!")
}

func TestRelayOverWs(t *testing.T) {
	msg := []byte("hello")
	// test tcp
	res := SendTcpMsg(msg, wsLocal)
	if string(res) != string(msg) {
		t.Fatal(res)
	}
	t.Log("test tcp over ws down!")
}

func BenchmarkTcpRelay(b *testing.B) {
	msg := []byte("hello")
	for i := 0; i <= b.N; i++ {
		res := SendTcpMsg(msg, rawLocal)
		if string(res) != string(msg) {
			b.Fatal(res)
		}
	}
}
