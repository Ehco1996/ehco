package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
)

var port = "0.0.0.0:1234"

func main() {
	conn, err := net.Dial("udp", port)
	if err != nil {
		fmt.Println("ERROR", err)
		os.Exit(1)
	}

	userInput := bufio.NewReader(os.Stdin)
	response := bufio.NewReader(conn)
	for {
		userLine, err := userInput.ReadBytes(byte('\n'))
		switch err {
		case nil:
			conn.Write(userLine)
		case io.EOF:
			os.Exit(0)
		default:
			fmt.Println("ERROR", err)
			os.Exit(1)
		}

		serverLine, err := response.ReadBytes(byte('\n'))
		switch err {
		case nil:
			fmt.Print(string(serverLine))
		case io.EOF:
			os.Exit(0)
		default:
			fmt.Println("ERROR", err)
			os.Exit(2)
		}
	}
}
