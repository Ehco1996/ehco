package cli

import (
	"github.com/Ehco1996/ehco/internal/constant"

	cli "github.com/urfave/cli/v2"
)

var (
	LocalAddr            string
	ListenType           constant.RelayType
	RemoteAddr           string
	TransportType        constant.RelayType
	ConfigPath           string
	WebPort              int
	WebToken             string
	EnablePing           bool
	SystemFilePath       = "/etc/systemd/system/ehco.service"
	LogLevel             string
	ConfigReloadInterval int
	BufferSize           int
)

var RootFlags = []cli.Flag{
	&cli.StringFlag{
		Name:        "l,local",
		Usage:       "监听地址，例如 0.0.0.0:1234",
		EnvVars:     []string{"EHCO_LOCAL_ADDR"},
		Destination: &LocalAddr,
	},
	&cli.StringFlag{
		Name:        "lt,listen_type",
		Value:       "raw",
		Usage:       "监听类型，可选项有 raw,ws,wss",
		EnvVars:     []string{"EHCO_LISTEN_TYPE"},
		Destination: (*string)(&ListenType),
		Required:    false,
	},
	&cli.StringFlag{
		Name:        "r,remote",
		Usage:       "转发地址，例如 0.0.0.0:5201，通过 ws 隧道转发时应为 ws://0.0.0.0:2443",
		EnvVars:     []string{"EHCO_REMOTE_ADDR"},
		Destination: &RemoteAddr,
	},
	&cli.StringFlag{
		Name:        "tt,transport_type",
		Value:       "raw",
		Usage:       "传输类型，可选选有 raw,ws,wss",
		EnvVars:     []string{"EHCO_TRANSPORT_TYPE"},
		Destination: (*string)(&TransportType),
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
		Destination: &LogLevel,
		DefaultText: "info",
	},
	&cli.IntFlag{
		Name:        "config_reload_interval",
		Usage:       "config reload interval",
		EnvVars:     []string{"EHCO_CONFIG_RELOAD_INTERVAL"},
		Destination: &ConfigReloadInterval,
		DefaultText: "60",
	},
	&cli.IntFlag{
		Name:        "buffer_size",
		Usage:       "set buffer size to when transport data default 20 * 1024(20KB)",
		EnvVars:     []string{"EHCO_BUFFER_SIZE"},
		Destination: &BufferSize,
	},
}
