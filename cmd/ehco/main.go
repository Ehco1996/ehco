package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/getsentry/sentry-go"
	cli "github.com/urfave/cli/v2"
	"go.uber.org/zap"

	"github.com/Ehco1996/ehco/internal/config"
	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/relay"
	"github.com/Ehco1996/ehco/internal/relay/conf"
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
			RelayConfigs: []*conf.Config{
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
		if err := cfg.Adjust(); err != nil {
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
		webS, err := web.NewServer(cfg)
		if err != nil {
			cmdLogger.Fatalf("NewWebServer meet err=%s", err.Error())
		}
		go func() {
			cmdLogger.Fatalf("StartWebServer meet err=%s", webS.Start())
		}()
	}

	if cfg.NeedStartXrayServer() {
		xrayS := xray.NewXrayServer(cfg)
		if err := xrayS.Setup(); err != nil {
			cmdLogger.Fatalf("Setup XrayServer meet err=%v", err)
		}
		if err := xrayS.Start(mainCtx); err != nil {
			cmdLogger.Fatalf("Start XrayServer meet err=%v", err)
		}
	}

	if cfg.NeedStartRelayServer() {
		web.EhcoAlive.Set(web.EhcoAliveStateRunning)
		rs, err := relay.NewServer(cfg)
		if err != nil {
			cmdLogger.Fatalf("NewRelayServer meet err=%s", err.Error())
		}
		go func() {
			cmdLogger.Fatalf("StartRelayServer meet err=%s", rs.Start(mainCtx))
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

	l, err := log.NewLogger("info")
	if err != nil {
		println("new info logger failed,err=", err)
		os.Exit(2)
	}
	cmdLogger = l.Sugar()

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
