package main

import (
	"github.com/Ehco1996/ehco/test"
)

func main() {
	var host = "0.0.0.0"
	var port = 9001
	test.RunEchoServer(host, port)
}
