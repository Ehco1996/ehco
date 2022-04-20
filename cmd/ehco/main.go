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
	"github.com/Ehco1996/ehco/pkg/xray"
)

var LocalAddr string
var ListenType string
var TCPRemoteAddr string
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

func createCliAPP() *cli.App {
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
			Name:        "l,local",
			Usage:       "监听地址，例如 0.0.0.0:1234",
			EnvVars:     []string{"EHCO_LOCAL_ADDR"},
			Destination: &LocalAddr,
			Required:    true,
		},
		&cli.StringFlag{
			Name:        "lt,listen_type",
			Value:       "raw",
			Usage:       "监听类型，可选项有 raw,ws,wss,mwss",
			EnvVars:     []string{"EHCO_LISTEN_TYPE"},
			Destination: &ListenType,
			Required:    false,
		},
		&cli.StringFlag{
			Name:        "r,remote",
			Usage:       "TCP 转发地址，例如 0.0.0.0:5201，通过 ws 隧道转发时应为 ws://0.0.0.0:2443",
			EnvVars:     []string{"EHCO_REMOTE_ADDR"},
			Destination: &TCPRemoteAddr,
		},
		&cli.StringFlag{
			Name:        "ur,udp_remote",
			Usage:       "UDP 转发地址，例如 0.0.0.0:1234",
			EnvVars:     []string{"EHCO_UDP_REMOTE_ADDR"},
			Destination: &UDPRemoteAddr,
		},
		&cli.StringFlag{
			Name:        "tt,transport_type",
			Value:       "raw",
			Usage:       "传输类型，可选选有 raw,ws,wss,mwss",
			EnvVars:     []string{"EHCO_TRANSPORT_TYPE"},
			Destination: &TransportType,
			Required:    false,
		},
		&cli.StringFlag{
			Name:        "c,config",
			Usage:       "配置文件地址，支持文件类型或 http api",
			EnvVars:     []string{"EHCO_CONFIG_FILE"},
			Destination: &ConfigPath,
		},
		&cli.IntFlag{
			Name:        "web_port",
			Usage:       "prometheus web expoter 的监听端口",
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
			Usage:       "如果访问webui时不带着正确的token，会直接reset连接",
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
	return app
}

func loadConfig() (cfg *config.Config, err error) {
	if ConfigPath != "" {
		cfg = config.NewConfigByPath(ConfigPath)
		if err := cfg.LoadConfig(); err != nil {
			return nil, err
		}
	} else {
		// prepare config from cli args
		cfg = &config.Config{
			WebPort:    WebPort,
			WebToken:   WebToken,
			EnablePing: EnablePing,
			PATH:       ConfigPath,
			RelayConfigs: []config.RelayConfig{
				{
					Listen:        LocalAddr,
					ListenType:    ListenType,
					TransportType: TransportType,
				},
			},
		}
		if TCPRemoteAddr != "" {
			cfg.RelayConfigs[0].TCPRemotes = []string{TCPRemoteAddr}
		}
		if UDPRemoteAddr != "" {
			cfg.RelayConfigs[0].UDPRemotes = []string{UDPRemoteAddr}
		}
		if err := cfg.Validate(); err != nil {
			return nil, err
		}
	}

	// init tls
	for _, cfg := range cfg.RelayConfigs {
		if cfg.ListenType == constant.Listen_WSS || cfg.ListenType == constant.Listen_MWSS ||
			cfg.TransportType == constant.Transport_WSS || cfg.TransportType == constant.Transport_MWSS {
			tls.InitTlsCfg()
			break
		}
	}
	return cfg, nil
}

func inArray(ele string, array []string) bool {
	for _, v := range array {
		if v == ele {
			return true
		}
	}
	return false
}

func startOneRelay(r *relay.Relay, relayM *sync.Map, errCh chan error) {
	relayM.Store(r.Name, r)
	if err := r.ListenAndServe(); err != nil && !errors.Is(err, net.ErrClosed) { // mute use closed network error
		errCh <- err
	}
}

func stopOneRelay(r *relay.Relay, relayM *sync.Map) {
	r.Close()
	relayM.Delete(r.Name)
}

func startRelayServers(ctx context.Context, cfg *config.Config) error {
	// relay ListenAddress -> relay
	relayM := &sync.Map{}
	errCH := make(chan error, 1)
	// init and relay servers
	for idx := range cfg.RelayConfigs {
		r, err := relay.NewRelay(&cfg.RelayConfigs[idx])
		if err != nil {
			return err
		}
		go startOneRelay(r, relayM, errCH)
	}
	// start watch config file TODO support reload from http , refine the ConfigPath global var
	if ConfigPath != "" {
		go watchAndReloadConfig(ctx, relayM, errCH)
	}

	select {
	case err := <-errCH:
		return err
	case <-ctx.Done():
		logger.Info("[relay]start to stop relay servers")
		relayM.Range(func(key, value interface{}) bool {
			r := value.(*relay.Relay)
			r.Close()
			return true
		})
		return nil
	}
}

func watchAndReloadConfig(ctx context.Context, relayM *sync.Map, errCh chan error) {
	logger.Errorf("[cfg] Start to watch config file: %s ", ConfigPath)

	reloadCH := make(chan os.Signal, 1)
	signal.Notify(reloadCH, syscall.SIGHUP)

	for {
		select {
		case <-ctx.Done():
			return
		case <-reloadCH:
			logger.Info("[cfg] Got A HUP Signal! Now Reloading Conf")
			newCfg, err := loadConfig()
			if err != nil {
				logger.Fatalf("[cfg] Reloading Conf meet error: %s ", err)
			}

			var newRelayAddrList []string
			for idx := range newCfg.RelayConfigs {
				r, err := relay.NewRelay(&newCfg.RelayConfigs[idx])
				if err != nil {
					logger.Fatalf("[cfg] reload new relay failed err=%s", err.Error())
				}
				newRelayAddrList = append(newRelayAddrList, r.Name)

				// reload old relay
				if oldR, ok := relayM.Load(r.Name); ok {
					oldR := oldR.(*relay.Relay)
					if oldR.Name != r.Name {
						logger.Infof("[cfg] close old relay name=%s", oldR.Name)
						stopOneRelay(oldR, relayM)
						go startOneRelay(r, relayM, errCh)
					}
					continue // no need to reload
				}
				// start bread new relay that not in old relayM
				logger.Infof("[cfg] starr new relay name=%s", r.Name)
				go startOneRelay(r, relayM, errCh)
			}
			// closed relay not in new config
			relayM.Range(func(key, value interface{}) bool {
				oldAddr := key.(string)
				if !inArray(oldAddr, newRelayAddrList) {
					v, _ := relayM.Load(oldAddr)
					oldR := v.(*relay.Relay)
					stopOneRelay(oldR, relayM)
				}
				return true
			})
		}
	}
}

func start(ctx *cli.Context) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	// init main ctx
	mainCtx, cancel := context.WithCancel(ctx.Context)
	defer cancel()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	if cfg.WebPort > 0 {
		go func() {
			logger.Fatalf("[web] StartWebServer meet err=%s", web.StartWebServer(cfg))
		}()
	}

	if cfg.XRayConfig != nil {
		go func() {
			s, err := xray.StartXrayServer(mainCtx, cfg)
			if err != nil {
				logger.Fatalf("[xray] StartXrayServer meet err=%s", err)
			}
			defer s.Close()

			if cfg.SyncTrafficEndPoint != "" {
				go func() {
					logger.Fatalf("[xray] StartSyncTask meet err=%s", xray.StartSyncTask(mainCtx, cfg))
				}()
			}
			<-ctx.Done()
		}()
	}

	if len(cfg.RelayConfigs) > 0 {
		go func() {
			logger.Fatalf("[relay] StartRelayServers meet err=%v", startRelayServers(mainCtx, cfg))
		}()
	}

	<-sigs
	return nil
}

func main() {
	app := createCliAPP()
	// register start command
	app.Action = start
	// main thread start
	if err := app.Run(os.Args); err != nil {
		logger.Fatal(err)
	}
}
