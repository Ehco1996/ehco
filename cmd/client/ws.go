package main

import (
	"flag"
	"github.com/gorilla/websocket"
	"log"
	"net/url"
	"os"
	"os/signal"
	"time"
)

var addr = flag.String("addr", "localhost:8080", "http service address")

func main() {
	flag.Parse()
	log.SetFlags(0)

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	u := url.URL{Scheme: "client", Host: *addr, Path: "/echo"}
	log.Printf("connecting to %s", u.String())

	c, _, err := websocket.DefaultDialer.Dial("ws://localhost:8080/echo", nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	for i := 0; i < 10; i++ {

		err = c.WriteMessage(websocket.BinaryMessage, []byte("123"))
		if err != nil {
			log.Println("write:", err)
		}

		_, message, _ := c.ReadMessage()
		log.Println("recv:", string(message))
		time.Sleep(time.Second)
	}

}
