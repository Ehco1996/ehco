package actions

import (
	"os"

	"github.com/Ehco1996/ehco/internal/config"
	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/logger"
	"github.com/Ehco1996/ehco/internal/relay"
	"github.com/Ehco1996/ehco/internal/tls"
	"github.com/Ehco1996/ehco/internal/web"
	"github.com/spf13/cobra"
)

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

var (
	LocalAddr      = os.Getenv("EHCO_LOCAL_ADDR")
	ListenType     string
	RemoteAddr     string
	UDPRemoteAddr  string
	TransportType  string
	ConfigPath     string
	WebfPort       string
	WebToken       string
	SystemFilePath = "/etc/systemd/system/ehco.service"

	ehcoCmd = &cobra.Command{
		Use:   "start",
		Short: "start ehco relay server",
		Long:  "ehco is a network relay tool and a typo :)",
		Run: func(cmd *cobra.Command, args []string) {
			start()
		},
	}
)

func init() {
	println(LocalAddr)

	ehcoCmd.PersistentFlags().StringVarP(&LocalAddr, "local", "l", LocalAddr, "监听地址,env:EHCO_LOCAL_ADDR")
	ehcoCmd.PersistentFlags().StringVarP(&LocalAddr, "listen_type", "lt", ListenType, "监听类型,env:EHCO_LISTEN_TYPE")
	ehcoCmd.PersistentFlags().StringVarP(&LocalAddr, "remote", "r", RemoteAddr, "转发地址,env:EHCO_REMOTE_ADDR")
	ehcoCmd.PersistentFlags().StringVarP(&LocalAddr, "udp_remote", "ur", UDPRemoteAddr, "UDP转发地址,env:EHCO_UDP_REMOTE_ADDR")
	ehcoCmd.PersistentFlags().StringVarP(&LocalAddr, "transport_type", "tt", TransportType, "监听地址,env:EHCO_TRANSPORT_TYPE")

	// ehcoCmd.PersistentFlags().StringVarP(&LocalAddr, "transport_type", "tt", TransportType, "监听地址,env:EHCO_TRANSPORT_TYPE")
	// ehcoCmd.PersistentFlags().StringVarP(&LocalAddr, "transport_type", "tt", TransportType, "监听地址,env:EHCO_TRANSPORT_TYPE")
	// ehcoCmd.PersistentFlags().StringVarP(&LocalAddr, "transport_type", "tt", TransportType, "监听地址,env:EHCO_TRANSPORT_TYPE")

	rootCmd.AddCommand(ehcoCmd)
}

// func main() {
// 	app := cli.NewApp()
// 	app.Name = "ehco"
// 	app.Version = constant.Version
// 	app.Usage = "ehco is a network relay tool and a typo :)"
// 	app.Flags = []cli.Flag{
// 		&cli.StringFlag{
// 		&cli.StringFlag{
// 			Name:        "c,config",
// 			Usage:       "配置文件地址",
// 			Destination: &ConfigPath,
// 		},
// 		&cli.StringFlag{
// 			Name:        "web_port",
// 			Usage:       "web监听端口",
// 			EnvVars:     []string{"EHCO_WEB_PORT"},
// 			Value:       "9000",
// 			Destination: &WebfPort,
// 		},
// 		&cli.StringFlag{
// 			Name:        "web_token",
// 			Usage:       "访问web的token",
// 			EnvVars:     []string{"EHCO_WEB_TOKEN"},
// 			Value:       "randomtoken",
// 			Destination: &WebToken,
// 		},
// 	}

// 	app.Commands = []*cli.Command{
// 		{
// 			Name:  "install",
// 			Usage: "install ehco systemd service",
// 			Action: func(c *cli.Context) error {
// 				fmt.Printf("Install ehco systemd file to `%s`\n", SystemFilePath)

// 				if _, err := os.Stat(SystemFilePath); err != nil && os.IsNotExist(err) {
// 					f, _ := os.OpenFile(SystemFilePath, os.O_CREATE|os.O_WRONLY, 0644)
// 					f.WriteString(SystemDTMPL)
// 					f.Close()
// 				}
// 				command := exec.Command("vi", SystemFilePath)
// 				command.Stdin = os.Stdin
// 				command.Stdout = os.Stdout
// 				return command.Run()
// 			},
// 		},
// 	}

// 	app.Action = start
// 	err := app.Run(os.Args)
// 	if err != nil {
// 		logger.Fatal(err)
// 	}

// }

func start() error {
	ch := make(chan error)
	var cfg *config.Config

	if ConfigPath != "" {
		cfg = config.NewConfigByPath(ConfigPath)
		if err := cfg.LoadConfig(); err != nil {
			logger.Fatal(err)
		}
	} else {
		cfg = &config.Config{
			PATH: ConfigPath,
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

	if WebfPort != "" {
		go web.StartWebServer(WebfPort, WebToken, cfg)
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
	return <-ch
}

func serveRelay(cfg config.RelayConfig, ch chan error) {
	r, err := relay.NewRelay(&cfg)
	if err != nil {
		logger.Fatal(err)
	}
	ch <- r.ListenAndServe()
}
