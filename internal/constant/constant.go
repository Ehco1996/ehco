package constant

import "time"

const (
	MaxMWSSStreamCnt = 10
	DialTimeOut      = 3 * time.Second
	MaxConKeepAlive  = 10 * time.Minute

	Listen_RAW  = "raw"
	Listen_WS   = "ws"
	Listen_WSS  = "wss"
	Listen_MWSS = "mwss"

	Transport_RAW  = "raw"
	Transport_WS   = "ws"
	Transport_WSS  = "wss"
	Transport_MWSS = "mwss"

	BUFFER_SIZE = 4 * 1024 // 4kb
)
