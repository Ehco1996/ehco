package test

import (
	"github.com/Ehco1996/ehco/internal/relay"
	"strconv"
	"testing"
)

var host = "0.0.0.0"
var port = 9002

var local = "0.0.0.0:1234"
var remote = host + ":" + strconv.Itoa(port)

func init() {
	// Start the new echo server.
	go RunEchoServer(host, port)

	// Start the relay server
	go func() {
		r, err := relay.NewRelay(local, remote)
		if err != nil {
			panic(err)
		}
		stop := make(chan error)
		stop <- r.ListenAndServe()
	}()
}

func TestRelay(t *testing.T) {
	msg := []byte("hello")
	// test tcp
	res := SendTcpMsg(msg, local)
	if string(res) != string(msg) {
		t.Fatal(res)
	}

	// test udp
	res = SendUdpMsg(msg, local)
	if string(res) != string(msg) {
		t.Fatal(res)
	}
}

func BenchmarkTcpRelay(b *testing.B) {
	msg := []byte("hello")
	for i := 0; i <= b.N; i++ {
		res := SendTcpMsg(msg, local)
		if string(res) != string(msg) {
			b.Fatal(res)
		}
	}
}
