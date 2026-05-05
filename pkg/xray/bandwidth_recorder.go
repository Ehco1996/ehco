package xray

import (
	"context"
	"time"

	psnet "github.com/shirou/gopsutil/v4/net"
)

// bandwidthRecorder samples host network counters via gopsutil and
// reports per-interval bandwidth (bytes/sec) plus the raw byte deltas.
// It replaces the older self-HTTP-scrape against /metrics, which only
// existed because the source (node_exporter) lived in the same process
// as the sink.
type bandwidthRecorder struct {
	currentSendBytes float64
	currentRecvBytes float64

	uploadBandwidthBytes   float64
	downloadBandwidthBytes float64

	lastRecordTime time.Time
}

func newBandwidthRecorder() *bandwidthRecorder {
	return &bandwidthRecorder{}
}

func (b *bandwidthRecorder) RecordOnce(ctx context.Context) (uploadIncr float64, downloadIncr float64, err error) {
	io, err := psnet.IOCountersWithContext(ctx, false)
	if err != nil {
		return 0, 0, err
	}
	var send, recv float64
	if len(io) > 0 {
		send = float64(io[0].BytesSent)
		recv = float64(io[0].BytesRecv)
	}

	now := time.Now()
	if !b.lastRecordTime.IsZero() {
		elapsed := now.Sub(b.lastRecordTime).Seconds()
		uploadIncr = send - b.currentSendBytes
		downloadIncr = recv - b.currentRecvBytes
		if elapsed > 0 {
			b.uploadBandwidthBytes = uploadIncr / elapsed
			b.downloadBandwidthBytes = downloadIncr / elapsed
		}
	}
	b.lastRecordTime = now
	b.currentSendBytes = send
	b.currentRecvBytes = recv
	return
}

func (b *bandwidthRecorder) GetDownloadBandwidth() float64 {
	return b.downloadBandwidthBytes
}

func (b *bandwidthRecorder) GetUploadBandwidth() float64 {
	return b.uploadBandwidthBytes
}
