package constant

import "time"

const (
	Version = "1.0.5"

	MaxMWSSStreamCnt = 10
	DialTimeOut      = 3 * time.Second
	MaxConKeepAlive  = 3 * time.Second

	Listen_RAW  = "raw"
	Listen_WS   = "ws"
	Listen_WSS  = "wss"
	Listen_MWSS = "mwss"

	Transport_RAW  = "raw"
	Transport_WS   = "ws"
	Transport_WSS  = "wss"
	Transport_MWSS = "mwss"

	BUFFER_SIZE = 4 * 1024 // 4kb

	IndexHTMLTMPL = `<!doctype html>
<html>
<head>
	<meta charset="UTF-8">
</head>
<body>
	<h2>Ehco(Version ` + Version + `)</h2>
	<h3>ehco is a network relay tool and a typo :)</h3>
	<p><a href="https://github.com/Ehco1996/ehco">More information here</a></p>

	<p><a href="/metrics/">Metrics</a></p>
	<p><a href="/debug/pprof/">Debug</a></p>
</body>
</html>
`
)
