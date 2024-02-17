package conn

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
)

func shortHashSHA256(input string) string {
	hasher := sha256.New()
	hasher.Write([]byte(input))
	hash := hasher.Sum(nil)
	return hex.EncodeToString(hash)[:7]
}

func connectionName(conn net.Conn) string {
	return fmt.Sprintf("l:<%s> r:<%s>", conn.LocalAddr(), conn.RemoteAddr())
}

func copyConn(conn1, conn2 *innerConn) error {
	errCH := make(chan error, 1)
	// copy conn1 to conn2,read from conn1 and write to conn2
	go func() {
		_, err := io.Copy(conn2, conn1)
		_ = conn2.CloseWrite() // all data is written to conn2 now, so close the write side of conn2 to send eof
		errCH <- err
	}()

	// reverse copy conn2 to conn1,read from conn2 and write to conn1
	_, err := io.Copy(conn1, conn2)
	_ = conn1.CloseWrite()

	err2 := <-errCH

	_ = conn1.CloseRead()
	_ = conn2.CloseRead()

	// handle errors, need to combine errors from both directions
	if err != nil && err2 != nil {
		err = fmt.Errorf("transport errors in both directions: %v, %v", err, err2)
	}
	if err != nil {
		return err
	}
	return err2
}
