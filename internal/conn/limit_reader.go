package conn

import (
	"io"
	"net"

	"github.com/juju/ratelimit"
)

type RateLimitedConn struct {
	net.Conn
	bucket *ratelimit.Bucket
	reader io.Reader
}

func NewRateLimitedConn(conn net.Conn, kbps int64) *RateLimitedConn {
	bps := float64(kbps) * 1000 // Convert kbps to bps (1 kbps = 1000 bps)
	rateBytesPerSec := bps / 8  // 1KB = 1024B, 1B = 8b
	bucket := ratelimit.NewBucketWithRate(rateBytesPerSec, int64(rateBytesPerSec))
	return &RateLimitedConn{
		Conn:   conn,
		bucket: bucket,
		reader: ratelimit.Reader(conn, bucket),
	}
}

func (r *RateLimitedConn) Read(p []byte) (int, error) {
	return r.reader.Read(p)
}
