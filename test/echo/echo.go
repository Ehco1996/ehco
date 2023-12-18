package echo

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"time"
)

func echo(conn net.Conn) {
	defer conn.Close()
	defer fmt.Println("conn closed", conn.RemoteAddr().String())
	buf := make([]byte, 10)
	for {
		i, err := conn.Read(buf)
		if err == io.EOF {
			fmt.Println("read eof")
			return
		}
		if err != nil {
			fmt.Println(err.Error())
			return
		}
		_, err = conn.Write(buf[:i])
		if err != nil {
			fmt.Println(err.Error())
			return
		}
	}
}

func ServeTcp(l net.Listener) {
	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println("accept err", err.Error())
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
	println("conn start", conn.RemoteAddr().String(), conn.LocalAddr().String())
	if _, err := conn.Write(msg); err != nil {
		log.Fatal(err)
	}
	time.Sleep(time.Second * 1)
	buf := make([]byte, len(msg))
	n, err := conn.Read(buf)
	if err != nil {
		log.Fatal(err)
	}
	conn.Close()
	println("conn closed", conn.RemoteAddr().String())
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
