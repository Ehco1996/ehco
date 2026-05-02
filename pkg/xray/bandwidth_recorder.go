package xray

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	netWorkSendMetric = "node_network_transmit_bytes_total"
	netWorkRecvMetric = "node_network_receive_bytes_total"
)

type bandwidthRecorder struct {
	currentSendBytes     float64
	uploadBandwidthBytes float64

	currentRecvBytes       float64
	downloadBandwidthBytes float64

	lastRecordTime time.Time

	httpClient *http.Client
	metricsURL string
	apiToken   string // optional bearer token; empty when web auth disabled
}

func NewBandwidthRecorder(metricsURL, apiToken string) *bandwidthRecorder {
	c := &http.Client{Timeout: 30 * time.Second}
	return &bandwidthRecorder{
		httpClient: c,
		metricsURL: metricsURL,
		apiToken:   apiToken,
	}
}

func (b *bandwidthRecorder) RecordOnce(ctx context.Context) (uploadIncr float64, downloadIncr float64, err error) {
	req, err := http.NewRequestWithContext(ctx, "GET", b.metricsURL, nil)
	if err != nil {
		return
	}
	if b.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+b.apiToken)
	}
	response, err := b.httpClient.Do(req)
	if err != nil {
		return
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return
	}
	lines := strings.Split(string(body), "\n")

	var send float64
	var recv float64

	for _, line := range lines {
		if strings.HasPrefix(line, netWorkSendMetric) {
			parts := strings.Split(line, " ")
			if len(parts) >= 2 {
				value := parts[1]
				send += parseFloat(value)
			}
		}

		if strings.HasPrefix(line, netWorkRecvMetric) {
			parts := strings.Split(line, " ")
			if len(parts) >= 2 {
				value := parts[1]
				recv += parseFloat(value)
			}
		}
	}

	now := time.Now()
	if !b.lastRecordTime.IsZero() {
		// calculate bandwidth
		elapsed := now.Sub(b.lastRecordTime).Seconds()
		uploadIncr = (send - b.currentSendBytes)
		downloadIncr = (recv - b.currentRecvBytes)
		if elapsed > 0 {
			b.uploadBandwidthBytes = uploadIncr / elapsed
			b.downloadBandwidthBytes = downloadIncr / elapsed
		}
	}
	b.lastRecordTime = now
	b.currentRecvBytes = recv
	b.currentSendBytes = send
	return
}

func parseFloat(s string) float64 {
	value, _ := strconv.ParseFloat(s, 64)
	return value
}

func (b *bandwidthRecorder) GetDownloadBandwidth() float64 {
	return b.downloadBandwidthBytes
}

func (b *bandwidthRecorder) GetUploadBandwidth() float64 {
	return b.uploadBandwidthBytes
}
