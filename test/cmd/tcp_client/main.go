package main

import "github.com/Ehco1996/ehco/test/echo"

func main() {
	msg := []byte("hello")
	addr := "127.0.0.1:2234"
	// addr := "127.0.0.1:2333"
	ret := echo.SendTcpMsg(msg, addr)
	println(string(ret))
}
