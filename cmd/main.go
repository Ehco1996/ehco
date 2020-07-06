package main

import (
	cli "github.com/urfave/cli/v2"
	"net/http"
	_ "net/http/pprof"
	"os"

	relay "github.com/Ehco1996/ehco/internal/relay"
)

var LocalAddr string
var ListenType string
var RemoteAddr string
var TransportType string
var ConfigPath string
var PprofPort string

func main() {
	app := cli.NewApp()
	app.Name = "ehco"
	app.Version = "0.1.1"
	app.Usage = "ehco is a network relay tool and a typo :)"
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:        "l, local",
			Value:       "0.0.0.0:1234",
			Usage:       "监听地址",
			EnvVars:     []string{"EHCO_LOCAL_ADDR"},
			Destination: &LocalAddr,
		},
		&cli.StringFlag{
			Name:        "lt,listen_type",
			Value:       "raw",
			Usage:       "监听类型",
			EnvVars:     []string{"EHCO_LISTEN_TYPE"},
			Destination: &ListenType,
			Required:    false,
		},
		&cli.StringFlag{
			Name:        "r,remote",
			Value:       "0.0.0.0:9001",
			Usage:       "转发地址",
			EnvVars:     []string{"EHCO_REMOTE_ADDR"},
			Destination: &RemoteAddr,
		},
		&cli.StringFlag{
			Name:        "tt,transport_type",
			Value:       "raw",
			Usage:       "传输类型",
			EnvVars:     []string{"EHCO_TRANSPORT_TYPE"},
			Destination: &TransportType,
			Required:    false,
		},
		&cli.StringFlag{
			Name:        "c,config",
			Usage:       "配置文件地址",
			Destination: &ConfigPath,
		},
		&cli.StringFlag{
			Name:        "pport",
			Usage:       "pprof监听端口",
			EnvVars:     []string{"EHCO_PPROF_PORT"},
			Destination: &PprofPort,
		},
	}

	app.Action = start
	err := app.Run(os.Args)
	if err != nil {
		relay.Logger.Fatal(err)
	}

}

func start(ctx *cli.Context) error {
	ch := make(chan error)
	if ConfigPath != "" {
		config := relay.NewConfig(ConfigPath)
		if err := config.LoadConfig(); err != nil {
			relay.Logger.Fatal(err)
		}

		initTls := false
		for _, cfg := range config.Configs {
			if !initTls && (cfg.ListenType == relay.Listen_WSS ||
				cfg.ListenType == relay.Listen_MWSS ||
				cfg.TransportType == relay.Transport_WSS ||
				cfg.TransportType == relay.Transport_MWSS) {
				initTls = true
				relay.InitTlsCfg()
			}
			go serveRelay(cfg.Listen, cfg.ListenType, cfg.Remote, cfg.TransportType, ch)
		}
	} else {
		if ListenType == relay.Listen_WSS ||
			ListenType == relay.Listen_MWSS ||
			TransportType == relay.Transport_WSS ||
			TransportType == relay.Transport_MWSS {
			relay.InitTlsCfg()
		}
		go serveRelay(LocalAddr, ListenType, RemoteAddr, TransportType, ch)
	}

	if PprofPort != "" {
		go func() {
			pps := "0.0.0.0:" + PprofPort
			relay.Logger.Infof("start pprof server at http://%s/debug/pprof/", pps)
			relay.Logger.Fatal(http.ListenAndServe(pps, nil))
		}()
	}

	return <-ch
}

func serveRelay(local, listenType, remote, transportType string, ch chan error) {
	r, err := relay.NewRelay(local, listenType, remote, transportType)
	if err != nil {
		relay.Logger.Fatal(err)
	}
	ch <- r.ListenAndServe()
}
