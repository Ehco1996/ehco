package xray

import (
	"context"
	"time"

	"github.com/shirou/gopsutil/v4/net"
)

type bandwidthRecorder struct {
	prevSend uint64
	prevRecv uint64
	primed   bool
}

func NewBandwidthRecorder() *bandwidthRecorder { return &bandwidthRecorder{} }

func (b *bandwidthRecorder) RecordOnce(ctx context.Context) (uploadIncr, downloadIncr float64, err error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	io, err := net.IOCountersWithContext(ctx, false)
	if err != nil {
		return 0, 0, err
	}
	if len(io) == 0 {
		return 0, 0, nil
	}

	send := io[0].BytesSent
	recv := io[0].BytesRecv

	if !b.primed {
		b.prevSend, b.prevRecv = send, recv
		b.primed = true
		return 0, 0, nil
	}
	if send >= b.prevSend {
		uploadIncr = float64(send - b.prevSend)
	}
	if recv >= b.prevRecv {
		downloadIncr = float64(recv - b.prevRecv)
	}
	b.prevSend, b.prevRecv = send, recv
	return
}
