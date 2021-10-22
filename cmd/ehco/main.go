package main

import (
	"context"
	"errors"
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
var WebPort int
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
	cli.VersionPrinter = func(c *cli.Context) {
		println("Welcome to ehco (ehco is a network relay tool and a typo)")
		println(fmt.Sprintf("Version=%s", constant.Version))
		println(fmt.Sprintf("GitBranch=%s", constant.GitBranch))
		println(fmt.Sprintf("GitRevision=%s", constant.GitRevision))
		println(fmt.Sprintf("BuildTime=%s", constant.BuildTime))
	}

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
			Destination: &WebPort,
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
	if cfg.WebPort > 0 {
		go web.StartWebServer(cfg)
	}
	return startAndWatchRelayServers(cfg)
}

func startAndWatchRelayServers(cfg *config.Config) error {
	// init main ctx
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// relay ListenAddress -> relay
	var relayM sync.Map

	// func used to start one relay
	var startOneRelayFunc = func(r *relay.Relay) {
		relayM.Store(r.Name, r)
		if err := r.ListenAndServe(); err != nil && !errors.Is(err, net.ErrClosed) {
			logger.Errorf("[relay] Name=%s ListenAndServe err=%s", r.Name, err)
		}
	}

	var stopOneRelayFunc = func(r *relay.Relay) {
		r.Close()
		relayM.Delete(r.Name)
	}

	// init relay map
	for idx := range cfg.Configs {
		r, err := relay.NewRelay(&cfg.Configs[idx])
		if err != nil {
			return err
		}
		go startOneRelayFunc(r)
	}
	// wg to control sub goroutine
	var wg sync.WaitGroup

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	wg.Add(1)
	go func(ctx context.Context) {
		defer wg.Done()
		select {
		case <-ctx.Done():
			logger.Info("ctx cancelled relay server exit")
		case <-sigs:
			relayM.Range(func(key, value interface{}) bool {
				r := value.(*relay.Relay)
				r.Close()
				return true
			})
			cancel()
		}
	}(ctx)

	// start reload loop
	reloadCH := make(chan os.Signal, 1)
	signal.Notify(reloadCH, syscall.SIGHUP)
	wg.Add(1)
	go func(ctx context.Context) {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case <-reloadCH:
				logger.Info("[cfg-reload] Got A HUP Signal! Now Reloading Conf ...")
				newCfg := loadConfig()

				var newRelayAddrList []string
				for idx := range newCfg.Configs {
					r, err := relay.NewRelay(&newCfg.Configs[idx])
					if err != nil {
						logger.Fatalf("[cfg-reload] reload new relay failed err=%s", err.Error())
					}
					newRelayAddrList = append(newRelayAddrList, r.Name)

					// reload old relay
					if oldR, ok := relayM.Load(r.Name); ok {
						oldR := oldR.(*relay.Relay)
						if oldR.Name != r.Name {
							logger.Infof("[cfg-reload] close old relay name=%s", oldR.Name)
							stopOneRelayFunc(oldR)
							go startOneRelayFunc(r)
						}
						continue // no need to reload
					}
					// start bread new relay that not in old relayM
					logger.Infof("[cfg-reload] starr new relay name=%s", r.Name)
					go startOneRelayFunc(r)
				}
				// closed relay not in new config
				relayM.Range(func(key, value interface{}) bool {
					oldAddr := key.(string)
					if !InArray(oldAddr, newRelayAddrList) {
						v, _ := relayM.Load(oldAddr)
						oldR := v.(*relay.Relay)
						stopOneRelayFunc(oldR)
					}
					return true
				})
			}
		}
	}(ctx)

	//TODO refine this
	web.EhcoAlive.Set(web.EhcoAliveStateRunning)
	wg.Wait()
	return nil
}

func loadConfig() (cfg *config.Config) {
	if ConfigPath != "" {
		cfg = config.NewConfigByPath(ConfigPath)
		if err := cfg.LoadConfig(); err != nil {
			logger.Fatal(err)
		}
	} else {
		cfg = &config.Config{
			WebPort:    WebPort,
			WebToken:   WebToken,
			EnablePing: EnablePing,
			PATH:       ConfigPath,
			Configs: []config.RelayConfig{
				{
					Listen:        LocalAddr,
					ListenType:    ListenType,
					TCPRemotes:    []string{RemoteAddr},
					TransportType: TransportType,
				},
			},
		}
		if UDPRemoteAddr != "" {
			cfg.Configs[0].UDPRemotes = []string{UDPRemoteAddr}
		}
	}

	// init tls
	for _, cfg := range cfg.Configs {
		if cfg.ListenType == constant.Listen_WSS || cfg.ListenType == constant.Listen_MWSS ||
			cfg.TransportType == constant.Transport_WSS || cfg.TransportType == constant.Transport_MWSS {
			tls.InitTlsCfg()
			break
		}
	}

	return cfg
}

func InArray(ele string, array []string) bool {
	for _, v := range array {
		if v == ele {
			return true
		}
	}
	return false
}
