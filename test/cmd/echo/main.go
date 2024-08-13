package main

import (
	"log"

	"github.com/Ehco1996/ehco/test/echo"
)

func main() {
	log.Println("start tcp.udp echo server at: 0.0.0.0:2333")
	es := echo.NewEchoServer("0.0.0.0", 2333)
	_ = es.Run()
}
