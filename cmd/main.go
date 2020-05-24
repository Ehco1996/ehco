package main

import (
	"log"
	"os"

	cli "github.com/urfave/cli/v2"

	relay "github.com/Ehco1996/ehco/internal/relay"
	web "github.com/Ehco1996/ehco/internal/web"
)

var LocalAddr string
var RemoteAddr string
var ConfigPath string

func main() {
	app := cli.NewApp()
	app.Name = "ehco"
	app.Version = "0.0.6"
	app.Usage = "A proxy used to relay tcp/udp traffic to anywhere"
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:        "l, local",
			Value:       "0.0.0.0:1234",
			Usage:       "监听地址",
			EnvVars:     []string{"EHCO_LOCAL_ADDR"},
			Destination: &LocalAddr,
		},
		&cli.StringFlag{
			Name:        "r,remote",
			Value:       "0.0.0.0:9001",
			Usage:       "转发地址",
			EnvVars:     []string{"EHCO_REMOTE_ADDR"},
			Destination: &RemoteAddr,
		},
		&cli.BoolFlag{
			Name:        "d,debug",
			Value:       false,
			Usage:       "转发地址",
			EnvVars:     []string{"EHCO_DEBUG"},
			Destination: &relay.DEBUG,
		},
		&cli.StringFlag{
			Name:        "c,config",
			Usage:       "配置文件地址",
			Destination: &ConfigPath,
		},
	}

	app.Action = start
	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}

}

func start(ctx *cli.Context) error {
	ch := make(chan error)
	if ConfigPath != "" {
		config := web.NewConfig(ConfigPath)
		if err := config.LoadConfig(); err != nil {
			log.Fatal(err)
		}

		for _, cfg := range config.Configs {
			go serveRelay(cfg.Listen, cfg.Remote, ch)
		}
	} else {
		go serveRelay(LocalAddr, RemoteAddr, ch)
	}
	return <-ch
}

func serveRelay(local string, remote string, ch chan error) {
	r, err := relay.NewRelay(local, remote)
	if err != nil {
		log.Fatal(err)
	}
	ch <- r.ListenAndServe()
}
