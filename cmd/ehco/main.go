package main

import (
	"net/http"
	_ "net/http/pprof"
	"os"

	cli "github.com/urfave/cli/v2"

	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/logger"
	"github.com/Ehco1996/ehco/internal/relay"
)

var LocalAddr string
var ListenType string
var RemoteAddr string
var UDPRemoteAddr string
var TransportType string
var ConfigPath string
var PprofPort string

func main() {
	app := cli.NewApp()
	app.Name = "ehco"
	app.Version = "0.1.6"
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
			Name:        "ur,udp_remote",
			Value:       "0.0.0.0:9001",
			Usage:       "UDP转发地址",
			EnvVars:     []string{"EHCO_UDP_REMOTE_ADDR"},
			Destination: &UDPRemoteAddr,
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
		logger.Logger.Fatal(err)
	}

}

func start(ctx *cli.Context) error {
	ch := make(chan error)
	var config *relay.Config

	if ConfigPath != "" {
		config = relay.NewConfigByPath(ConfigPath)
		if err := config.LoadConfig(); err != nil {
			logger.Logger.Fatal(err)
		}
	} else {
		config = &relay.Config{
			PATH: ConfigPath,
			Configs: []relay.RelayConfig{
				{
					Listen:        LocalAddr,
					ListenType:    ListenType,
					TCPRemotes:    []string{RemoteAddr},
					UDPRemotes:    []string{UDPRemoteAddr},
					TransportType: TransportType,
				},
			},
		}
	}
	initTls := false
	for _, cfg := range config.Configs {
		if !initTls && (cfg.ListenType == constant.Listen_WSS ||
			cfg.ListenType == constant.Listen_MWSS ||
			cfg.TransportType == constant.Transport_WSS ||
			cfg.TransportType == constant.Transport_MWSS) {
			initTls = true
			relay.InitTlsCfg()
		}
		go serveRelay(cfg, ch)
	}

	// start debug pprof server
	if PprofPort != "" {
		go func() {
			pps := "0.0.0.0:" + PprofPort
			logger.Logger.Infof("start pprof server at http://%s/debug/pprof/", pps)
			logger.Logger.Fatal(http.ListenAndServe(pps, nil))
		}()
	}

	return <-ch
}

func serveRelay(cfg relay.RelayConfig, ch chan error) {
	r, err := relay.NewRelay(&cfg)
	if err != nil {
		logger.Logger.Fatal(err)
	}
	ch <- r.ListenAndServe()
}
