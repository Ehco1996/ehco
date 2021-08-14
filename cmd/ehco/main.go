package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"

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
LimitNOFILE=65535
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
	cfg := loadConfig()
	initTls(cfg)

	if cfg.WebPort > 0 {
		go web.StartWebServer(cfg)
	}
	return startAndWatchRelayServers(cfg.Configs)
}

func startAndWatchRelayServers(relayConfigList []config.RelayConfig) error {
	// relay name -> relay
	relayM := make(map[string]*relay.Relay)
	for idx := range relayConfigList {
		r, err := relay.NewRelay(&relayConfigList[idx])
		if err != nil {
			return err
		}
		relayM[r.Name] = r
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for name := range relayM {
		go func(name string) {
			err := relayM[name].ListenAndServe()
			if err != nil {
				if _, ok := err.(*net.OpError); !ok {
					logger.Errorf("[relay] name=%s ListenAndServe err=%s", name, err)
				}
			}
			cancel()
		}(name)
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	var wg sync.WaitGroup
	wg.Add(1)
	go func(ctx context.Context) {
		defer wg.Done()

		select {
		case <-ctx.Done():
			logger.Info("Relay server exit")
		case <-sigs:
			for name := range relayM {
				logger.Infof("[relay] Stop %s ...", name)
				relayM[name].Close()
			}
		}
	}(ctx)

	wg.Wait()
	return nil
}

func initTls(cfg *config.Config) {
	for _, cfg := range cfg.Configs {
		if cfg.ListenType == constant.Listen_WSS || cfg.ListenType == constant.Listen_MWSS ||
			cfg.TransportType == constant.Transport_WSS || cfg.TransportType == constant.Transport_MWSS {
			tls.InitTlsCfg()
			break
		}
	}
}

func loadConfig() (cfg *config.Config) {
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
	return cfg
}
