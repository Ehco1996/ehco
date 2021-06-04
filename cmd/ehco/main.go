package main

import (
	"fmt"
	"os"
	"os/exec"

	cli "github.com/urfave/cli/v2"

	"github.com/Ehco1996/ehco/internal/config"
	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/logger"
	"github.com/Ehco1996/ehco/internal/relay"
	"github.com/Ehco1996/ehco/internal/tls"
	"github.com/Ehco1996/ehco/internal/web"
)

var LocalAddr string
var ListenType string
var RemoteAddr string
var UDPRemoteAddr string
var TransportType string
var ConfigPath string
var WebfPort int
var WebToken string
var EnablePing bool
var SystemFilePath = "/etc/systemd/system/ehco.service"

const SystemDTMPL = `# Ehco service
[Unit]
Description=ehco
After=network.target

[Service]
ExecStart=/root/ehco -c ""
Restart=always

[Install]
WantedBy=multi-user.target
`

func main() {
	app := cli.NewApp()
	app.Name = "ehco"
	app.Version = constant.Version
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
		&cli.IntFlag{
			Name:        "web_port",
			Usage:       "promtheus web expoter 的监听端口",
			EnvVars:     []string{"EHCO_WEB_PORT"},
			Value:       0,
			Destination: &WebfPort,
		},
		&cli.BoolFlag{
			Name:        "enable_ping",
			Usage:       "是否打开 ping metrics",
			EnvVars:     []string{"EHCO_ENABLE_PING"},
			Value:       true,
			Destination: &EnablePing,
		},
		&cli.StringFlag{
			Name:        "web_token",
			Usage:       "访问web的token,如果访问不带着正确的token，会直接reset连接",
			EnvVars:     []string{"EHCO_WEB_TOKEN"},
			Destination: &WebToken,
		},
	}

	app.Commands = []*cli.Command{
		{
			Name:  "install",
			Usage: "install ehco systemd service",
			Action: func(c *cli.Context) error {
				fmt.Printf("Install ehco systemd file to `%s`\n", SystemFilePath)

				if _, err := os.Stat(SystemFilePath); err != nil && os.IsNotExist(err) {
					f, _ := os.OpenFile(SystemFilePath, os.O_CREATE|os.O_WRONLY, 0644)
					if _, err := f.WriteString(SystemDTMPL); err != nil {
						logger.Fatal(err)
					}
					f.Close()
				}

				command := exec.Command("vi", SystemFilePath)
				command.Stdin = os.Stdin
				command.Stdout = os.Stdout
				return command.Run()
			},
		},
	}

	app.Action = start
	err := app.Run(os.Args)
	if err != nil {
		logger.Fatal(err)
	}

}

func start(ctx *cli.Context) error {
	ch := make(chan error)
	var cfg *config.Config

	if ConfigPath != "" {
		cfg = config.NewConfigByPath(ConfigPath)
		if err := cfg.LoadConfig(); err != nil {
			logger.Fatal(err)
		}
	} else {
		cfg = &config.Config{
			WebPort:    WebfPort,
			WebToken:   WebToken,
			EnablePing: EnablePing,
			PATH:       ConfigPath,
			Configs: []config.RelayConfig{
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
	for _, cfg := range cfg.Configs {
		if !initTls && (cfg.ListenType == constant.Listen_WSS ||
			cfg.ListenType == constant.Listen_MWSS ||
			cfg.TransportType == constant.Transport_WSS ||
			cfg.TransportType == constant.Transport_MWSS) {
			initTls = true
			tls.InitTlsCfg()
		}
		go serveRelay(cfg, ch)
	}
	go web.StartWebServer(cfg)
	return <-ch
}

func serveRelay(cfg config.RelayConfig, ch chan error) {
	r, err := relay.NewRelay(&cfg)
	if err != nil {
		logger.Fatal(err)
	}
	ch <- r.ListenAndServe()
}
