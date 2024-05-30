package main

import (
	"time"

	"github.com/Ehco1996/ehco/test/echo"
)

func main() {
	msg := []byte("hello")

	echoServerAddr := "127.0.0.1:2333"
	relayAddr := "127.0.0.1:2234"
	println("real echo server at:", echoServerAddr, "relay addr:", relayAddr)

	ret := echo.SendTcpMsg(msg, relayAddr)
	if string(ret) != "hello" {
		panic("relay short failed")
	}
	println("test short conn success, hello sended and received")

	if err := echo.EchoTcpMsgLong(msg, time.Second, relayAddr); err != nil {
		panic("relay long failed:" + err.Error())
	}
	println("test long conn success")
}
