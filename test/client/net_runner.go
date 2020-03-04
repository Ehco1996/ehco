package main

import (
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

var address = "0.0.0.0:1234"

func sendTcpMsg(msg []byte, wg *sync.WaitGroup) {
	defer wg.Done()

	conn, err := net.Dial("tcp", address)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	conn.Write(msg)
	buf := make([]byte, 100)
	time.Sleep(time.Second * 1)
	conn.Read(buf)
	log.Printf("msg: %s", string(buf))
}

func sendUdpMsg(msg []byte, wg *sync.WaitGroup) {
	defer wg.Done()

	conn, err := net.Dial("udp", address)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	conn.Write(msg)
	buf := make([]byte, 100)
	time.Sleep(time.Second * 1)
	conn.Read(buf)
	log.Printf("msg: %s", string(buf))
}

func main() {
	base := strings.Repeat("hellohellohellohello", 100)
	var wg sync.WaitGroup

	for {
		for i := 0; i < 50; i++ {
			msg := string(i) + "+++" + base
			wg.Add(2)
			log.Print(i)
			go sendUdpMsg([]byte(msg), &wg)
			go sendTcpMsg([]byte(msg), &wg)
		}
		wg.Wait()
	}

}
