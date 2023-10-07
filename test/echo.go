package test

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"time"
)

// TODO support random timeouts
func echo(conn net.Conn) {
	defer conn.Close()
	defer fmt.Println("conn closed", conn.RemoteAddr().String())

	for {
		buf := make([]byte, 512)
		i, err := conn.Read(buf)
		if err == io.EOF {
			return
		}
		if err != nil {
			fmt.Println(err)
			continue
		}
		_, err = conn.Write(buf[:i])
		if err != nil {
			continue
		}
	}
}

func ServeTcp(l net.Listener) {
	for {
		conn, err := l.Accept()
		if err != nil {
			continue
		}
		go echo(conn)
	}
}

func ServeUdp(conn *net.UDPConn) {
	buf := make([]byte, 1500)
	for {
		number, remote, err := conn.ReadFromUDP(buf)
		if err != nil {
			fmt.Printf("net.ReadFromUDP() error: %s\n", err)
		}
		_, writeErr := conn.WriteTo(buf[0:number], remote)
		if writeErr != nil {
			fmt.Printf("net.WriteTo() error: %s\n", writeErr)
		}
	}
}

func RunEchoServer(host string, port int) {
	var err error
	tcpAddr := host + ":" + strconv.Itoa(port)
	l, err := net.Listen("tcp", tcpAddr)
	defer func() {
		err = l.Close()
		if err != nil {
			fmt.Println(err.Error())
		}
	}()

	if err != nil {
		fmt.Println("ERROR", err)
		os.Exit(1)
	}

	udpAddr := net.UDPAddr{Port: port, IP: net.ParseIP(host)}
	udpConn, err := net.ListenUDP("udp", &udpAddr)
	defer func() {
		err = udpConn.Close()
		if err != nil {
			fmt.Println(err.Error())
		}
	}()

	if err != nil {
		fmt.Println("ERROR", err)
		os.Exit(1)
	}

	fmt.Println("start echo server at:", tcpAddr)
	stop := make(chan error)
	go ServeTcp(l)
	go ServeUdp(udpConn)
	<-stop
}

func SendTcpMsg(msg []byte, address string) []byte {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write(msg); err != nil {
		log.Fatal(err)
	}
	buf := make([]byte, len(msg))
	time.Sleep(time.Second * 1)
	n, _ := conn.Read(buf)
	return buf[:n]
}

func SendUdpMsg(msg []byte, address string) []byte {
	conn, err := net.Dial("udp", address)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write(msg); err != nil {
		log.Fatal(err)
	}
	buf := make([]byte, len(msg))
	time.Sleep(time.Second * 1)
	n, _ := conn.Read(buf)
	return buf[:n]
}
