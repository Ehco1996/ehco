package main

import "github.com/Ehco1996/ehco/test/echo"

func main() {
	msg := []byte("hello")

	echoServerAddr := "127.0.0.1:2333"
	println("real echo server at:", echoServerAddr)

	// start ehco real server
	// go run cmd/ehco/main.go -l 0.0.0.0:2234 -r 0.0.0.0:2333

	relayAddr := "127.0.0.1:2234"
	ret := echo.SendTcpMsg(msg, relayAddr)
	println(string(ret))
}
