package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
)

var host = "0.0.0.0"
var port = 9002

func echo(conn net.Conn) {
	defer conn.Close()
	defer fmt.Println("conn closed", conn.RemoteAddr().String())

	fmt.Printf("Connected to: %s\n", conn.RemoteAddr().String())

	for {
		buf := make([]byte, 512)
		_, err := conn.Read(buf)
		if err == io.EOF {
			fmt.Println("Eof reading")
			return
		}
		if err != nil {
			fmt.Println("Error reading:")
			fmt.Println(err)
			continue
		}

		fmt.Println(fmt.Sprintf("[%s]", conn.RemoteAddr().String()), string(buf))

		_, err = conn.Write(buf)
		if err != nil {
			fmt.Println("Error writing:")
			fmt.Println(err)
			continue
		}
	}
}

func serveTcp(l net.Listener) {
	for {
		conn, err := l.Accept()
		fmt.Println(conn)
		if err != nil {
			fmt.Println("ERROR", err)
			continue
		}
		go echo(conn)
	}
}

func serveUdp(conn *net.UDPConn) {

	buf := make([]byte, 1500)
	for {

		number, remote, err := conn.ReadFromUDP(buf)
		if err != nil {
			fmt.Printf("net.ReadFromUDP() error: %s\n", err)
		} else {
			fmt.Printf("Read %d bytes from socket\n", number)
			fmt.Printf("Bytes: %q\n", string(buf[:number]))
		}
		fmt.Printf("Remote address: %v\n", remote)

		number, writeErr := conn.WriteTo(buf[0:number], remote)
		if writeErr != nil {
			fmt.Printf("net.WriteTo() error: %s\n", writeErr)
		} else {
			fmt.Printf("Wrote %d bytes to socket\n", number)
		}
	}
	fmt.Printf("Out of infinite loop\n")
}

func main() {
	var err error
	tcpAddr := host + ":" + strconv.Itoa(port)
	l, err := net.Listen("tcp", tcpAddr)
	if err != nil {
		fmt.Println("ERROR", err)
		os.Exit(1)
	}

	udpAddr := net.UDPAddr{Port: port, IP: net.ParseIP(host)}
	udpConn, err := net.ListenUDP("udp", &udpAddr)
	if err != nil {
		fmt.Println("ERROR", err)
		os.Exit(1)
	}

	fmt.Println("start echo server at:", tcpAddr)
	stop := make(chan error)
	go serveTcp(l)
	go serveUdp(udpConn)
	<-stop
}
