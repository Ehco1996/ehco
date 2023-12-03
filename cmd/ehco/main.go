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
	"time"

	"github.com/getsentry/sentry-go"
	cli "github.com/urfave/cli/v2"
	"go.uber.org/zap"

	"github.com/Ehco1996/ehco/internal/config"
	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/relay"
	"github.com/Ehco1996/ehco/internal/tls"
	"github.com/Ehco1996/ehco/internal/web"
	"github.com/Ehco1996/ehco/pkg/log"
	"github.com/Ehco1996/ehco/pkg/xray"
)

// flags
var (
	LocalAddr            string
	ListenType           string
	TCPRemoteAddr        string
	UDPRemoteAddr        string
	TransportType        string
	ConfigPath           string
	WebPort              int
	WebToken             string
	EnablePing           bool
	SystemFilePath       = "/etc/systemd/system/ehco.service"
	LogLevel             string
	ConfigReloadInterval int
)

var cmdLogger *zap.SugaredLogger

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
			Usage:       "prometheus web exporter 的监听端口",
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
		&cli.StringFlag{
			Name:        "log_level",
			Usage:       "log level",
			EnvVars:     []string{"EHCO_LOG_LEVEL"},
			Destination: &WebToken,
			DefaultText: "info",
		},
		&cli.IntFlag{
			Name:        "config_reload_interval",
			Usage:       "config reload interval",
			EnvVars:     []string{"EHCO_CONFIG_RELOAD_INTERVAL"},
			Destination: &ConfigReloadInterval,
			DefaultText: "60",
		},
	}

	app.Commands = []*cli.Command{
		{
			Name:  "install",
			Usage: "install ehco systemd service",
			Action: func(c *cli.Context) error {
				fmt.Printf("Install ehco systemd file to `%s`\n", SystemFilePath)
				if _, err := os.Stat(SystemFilePath); err != nil && os.IsNotExist(err) {
					f, _ := os.OpenFile(SystemFilePath, os.O_CREATE|os.O_WRONLY, 0o644)
					if _, err := f.WriteString(SystemDTMPL); err != nil {
						cmdLogger.Fatal(err)
					}
					return f.Close()
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
		cfg = config.NewConfig(ConfigPath)
		if err := cfg.LoadConfig(); err != nil {
			return nil, err
		}
	} else {
		cfg = &config.Config{
			WebPort:        WebPort,
			WebToken:       WebToken,
			EnablePing:     EnablePing,
			PATH:           ConfigPath,
			LogLeveL:       LogLevel,
			ReloadInterval: ConfigReloadInterval,
			RelayConfigs: []*config.RelayConfig{
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
			if err := tls.InitTlsCfg(); err != nil {
				return nil, err
			}
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
		r, err := relay.NewRelay(cfg.RelayConfigs[idx])
		if err != nil {
			return err
		}
		go startOneRelay(r, relayM, errCH)
	}
	// start watch config file TODO support reload from http , refine the ConfigPath global var
	if ConfigPath != "" {
		go watchAndReloadRelayConfig(ctx, cfg, relayM, errCH)
	}

	select {
	case err := <-errCH:
		return err
	case <-ctx.Done():
		cmdLogger.Info("start to stop relay servers")
		relayM.Range(func(key, value interface{}) bool {
			r := value.(*relay.Relay)
			r.Close()
			return true
		})
		return nil
	}
}

func watchAndReloadRelayConfig(ctx context.Context, curCfg *config.Config, relayM *sync.Map, errCh chan error) {
	cmdLogger.Infof("Start to watch config file: %s ", ConfigPath)
	reladRelay := func() error {
		newCfg, err := loadConfig()
		if err != nil {
			cmdLogger.Errorf("Reloading Realy Conf meet error: %s ", err)
			return err
		}
		var newRelayAddrList []string
		for idx := range newCfg.RelayConfigs {
			r, err := relay.NewRelay(newCfg.RelayConfigs[idx])
			if err != nil {
				cmdLogger.Errorf("reload new relay failed err=%s", err.Error())
				return err
			}
			newRelayAddrList = append(newRelayAddrList, r.Name)
			// reload relay when name change
			if oldR, ok := relayM.Load(r.Name); ok {
				oldR := oldR.(*relay.Relay)
				if oldR.Name != r.Name {
					cmdLogger.Infof("close old relay name=%s", oldR.Name)
					stopOneRelay(oldR, relayM)
					go startOneRelay(r, relayM, errCh)
				}
				continue // no need to reload
			}
			// start bread new relay that not in old relayM
			cmdLogger.Infof("starr new relay name=%s", r.Name)
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
		return nil
	}

	reloadCH := make(chan struct{}, 1)

	// listen syscall.SIGHUP to trigger reload
	sigHubCH := make(chan os.Signal, 1)
	signal.Notify(sigHubCH, syscall.SIGHUP)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-sigHubCH:
				cmdLogger.Info("Now Reloading Relay Conf By HUP Signal! ")
				reloadCH <- struct{}{}
			}
		}
	}()

	// ticker to reload config
	if curCfg.ReloadInterval > 0 {
		ticker := time.NewTicker(time.Second * time.Duration(curCfg.ReloadInterval))
		defer ticker.Stop()
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					cmdLogger.Info("Now Reloading Relay Conf By Ticker! ")
					reloadCH <- struct{}{}
				}
			}
		}()
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-reloadCH:
			if err := reladRelay(); err != nil {
				cmdLogger.Errorf("Reloading Relay Conf meet error: %s ", err)
				errCh <- err
			}
		}
	}
}

func initSentry() error {
	if dsn := os.Getenv("SENTRY_DSN"); dsn != "" {
		cmdLogger.Infof("init sentry with dsn", dsn)
		return sentry.Init(sentry.ClientOptions{Dsn: dsn})
	}
	return nil
}

func initLogger(cfg *config.Config) error {
	if err := log.InitGlobalLogger(cfg.LogLeveL); err != nil {
		return err
	}
	return nil
}

func start(ctx *cli.Context) error {
	cmdLogger = log.MustNewInfoLogger("cmd")

	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if err := initLogger(cfg); err != nil {
		return err
	}

	if err := initSentry(); err != nil {
		return err
	}

	// init main ctx
	mainCtx, cancel := context.WithCancel(ctx.Context)
	defer cancel()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	if cfg.NeedStartWebServer() {
		go func() {
			cmdLogger.Fatalf("StartWebServer meet err=%s", web.StartWebServer(cfg))
		}()
	}

	if cfg.XRayConfig != nil {
		xrayS, err := xray.NewXrayServer(cfg)
		if err != nil {
			return err
		}
		if err := xrayS.Start(mainCtx); err != nil {
			cmdLogger.Fatalf("StartXrayServer meet err=%v", err)
		}
	}

	if len(cfg.RelayConfigs) > 0 {
		go func() {
			cmdLogger.Fatalf("StartRelayServers meet err=%v", startRelayServers(mainCtx, cfg))
		}()
	}

	<-sigs
	return nil
}

func main() {
	defer func() {
		err := recover()
		if err != nil {
			sentry.CurrentHub().Recover(err)
			sentry.Flush(time.Second * 5)
		}
	}()

	app := createCliAPP()
	// register start command
	app.Action = start
	// main thread start
	if err := app.Run(os.Args); err != nil {
		sentry.CurrentHub().CaptureException(err)
		sentry.Flush(time.Second * 5)
		cmdLogger.Fatal("start ehco server failed,err=", err)
	}
}
