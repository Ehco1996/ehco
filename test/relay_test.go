package test

import (
	"github.com/Ehco1996/ehco/internal/relay"
	"net"
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

	// test tcp
	sendMsg := []byte("hello")
	c, err := net.Dial("tcp", local)
	if err != nil {
		t.Fatal(err)
	}
	c.Write(sendMsg)
	tcpRes := make([]byte, len(sendMsg))
	c.Read(tcpRes)

	if string(tcpRes) != string(sendMsg) {
		t.Fatal(tcpRes)
	}
}
