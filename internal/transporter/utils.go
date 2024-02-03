package transporter

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
