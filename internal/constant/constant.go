package constant

import "time"

var (
	// allow change in test
	IdleTimeOut = 10 * time.Second

	Version     = "1.1.4-dev"
	GitBranch   string
	GitRevision string
	BuildTime   string
)

const (
	DialTimeOut = 3 * time.Second

	SmuxGCDuration       = 30 * time.Second
	SmuxMaxAliveDuration = 10 * time.Minute
	SmuxMaxStreamCnt     = 5

	Listen_RAW  = "raw"
	Listen_WS   = "ws"
	Listen_WSS  = "wss"
	Listen_MWSS = "mwss"
	Listen_MTCP = "mtcp"

	Transport_RAW  = "raw"
	Transport_WS   = "ws"
	Transport_WSS  = "wss"
	Transport_MWSS = "mwss"
	Transport_MTCP = "mtcp"

	// todo add udp buffer size
	BUFFER_POOL_SIZE = 1024      // support 512 connections
	BUFFER_SIZE      = 20 * 1024 // 20KB the maximum packet size of shadowsocks is about 16 KiB
)

const WelcomeHTML = `
<!doctype html>
<html>

<head>
	<meta charset="UTF-8">
	<style>
		body {
			display: flex;
			flex-direction: column;
			align-items: center; /* Center layout */
			font-family: Arial, sans-serif;
			text-align: left; /* Left-aligned text */
			padding: 2em;
		}
		.container {
			width: 60%; /* Control width */
			lex-grow: 1;
		}

		.build-info, .links {
			margin-bottom: 30px;
			border: 1px solid black;
			padding: 10px;
			list-style-type: none;
		}

		.build-info h3 {
			margin: 10px 0;
		}

		a {
			text-decoration: none;
			color: blue;
		}

		#reloadButton {
			margin-top: 30px;
			padding: 10px 20px;
			text-align: center;
		}
		.btn-container {
			text-align: center;
			width: 100%;
		}
		footer {
			text-align: center;
			margin-top: 20px;
		}
	</style>
</head>
<body>
	<div class="container">
		<h1>ehco is a network relay tool and a typo :)</h1>
		<div class="build-info">
			<h2>Build Info:</h2>
			<h3>Version: {{.Version}}</h3>
			<h3>GitBranch: {{.GitBranch}}</h3>
			<h3>GitRevision: {{.GitRevision}}</h3>
			<h3>BuildTime: {{.BuildTime}}</h3>
		</div>

		<div class="links">
			<h3>Links:</h3>
			<ul>
				<li><a href="/metrics/">Metrics</a></li>
				<li><a href="/debug/pprof/">Debug</a></li>
				<li><a href="/config/">Current Config</a></li>
			</ul>

			{{if .SubConfigs}}
			<h3>Clash Providers:</h3>
			<ul>
				{{range .SubConfigs}}
					<li><a href="/clash_proxy_provider/?sub_name={{.Name}}">{{.Name}}</a></li>
					<li><a href="/clash_proxy_provider/group-by-prefix/?sub_name={{.Name}}">{{.Name}}-grouped</a></li>
				{{end}}
			</ul>
			{{end}}
		</div>

		<div class="btn-container">
		<button id="reloadButton">Reload Config</button>
		</div>

		<footer>
		<a href="https://github.com/Ehco1996/ehco">Source code</a>
		</footer>
	</div>
</body>


<script>
document.getElementById("reloadButton").addEventListener("click", function() {
	var request = new XMLHttpRequest();
	request.open("POST", "/reload/");
	request.onreadystatechange = function() {
		if (request.readyState === XMLHttpRequest.DONE) {
			if (request.status === 200) {
				msg = "Reload config success." + "response: " + request.responseText;
				alert(msg);
			} else {
				msg = "Failed to reload config." + "response: " + request.responseText;
				alert(msg);;
			}
		}
	};
	request.send();
});
</script>
</html>
`
