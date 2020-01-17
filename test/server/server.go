package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
)

var addr = "0.0.0.0:9001"

func echo(conn net.Conn) {
	r := bufio.NewReader(conn)
	//conn.SetDeadline(time.Now().Add(time.Duration(1) * time.Second))
	for {
		line, err := r.ReadBytes(byte('\n'))
		fmt.Println(line)
		if len(line) == 0 {
			break
		}
		switch err {
		case nil:
			break
		case io.EOF:
		default:
			fmt.Println("ECHO ERROR", err)
			break
		}
		conn.Write(line)
	}
	conn.Close()
}

func main() {
	l, err := net.Listen("tcp", addr)
	fmt.Println("start echo server at:", addr)
	if err != nil {
		fmt.Println("ERROR", err)
		os.Exit(1)
	}

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
