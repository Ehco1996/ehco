package constant

import "time"

var (
	DefaultDeadline = 30 * time.Second

	Version     = "1.1.2"
	GitBranch   string
	GitRevision string
	BuildTime   string

	IndexHTMLTMPL = `<!doctype html>
	<html>
	<head>
		<meta charset="UTF-8">
	</head>
	<body>
		<h2>ehco is a network relay tool and a typo :)</h2>
		<hr>
		<h3>Version: ` + Version + `</h3>
		<h3>GitBranch: ` + GitBranch + `</h3>
		<h3>GitRevision: ` + GitRevision + `</h3>
		<h3>BuildTime: ` + BuildTime + `</h3>
		<hr>
		<p><a href="https://github.com/Ehco1996/ehco">More information here</a></p>
		<p><a href="/metrics/">Metrics</a></p>
		<p><a href="/debug/pprof/">Debug</a></p>
	</body>
	</html>
	`
)

const (
	MaxMWSSStreamCnt = 100
	DialTimeOut      = 3 * time.Second

	Listen_RAW  = "raw"
	Listen_WS   = "ws"
	Listen_WSS  = "wss"
	Listen_MWSS = "mwss"

	Transport_RAW  = "raw"
	Transport_WS   = "ws"
	Transport_WSS  = "wss"
	Transport_MWSS = "mwss"

	BUFFER_POOL_SIZE = 128
	BUFFER_SIZE      = 128 * 1024 // 128KB
)
