package cli

import (
	"context"
	"os"

	"github.com/Ehco1996/ehco/internal/config"
	"github.com/Ehco1996/ehco/internal/constant"
	"github.com/Ehco1996/ehco/internal/metrics"
	"github.com/Ehco1996/ehco/internal/relay"
	"github.com/Ehco1996/ehco/internal/relay/conf"
	"github.com/Ehco1996/ehco/internal/tls"
	"github.com/Ehco1996/ehco/internal/web"
	"github.com/Ehco1996/ehco/pkg/log"
	"github.com/Ehco1996/ehco/pkg/xray"
	"github.com/getsentry/sentry-go"
)

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
		cliLogger.Infof("init sentry with dsn:%s", dsn)
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

func InitConfigAndComponents() (*config.Config, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}
	if err := initLogger(cfg); err != nil {
		return nil, err
	}
	if err := initSentry(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func MustStartComponents(mainCtx context.Context, cfg *config.Config) {
	cliLogger.Infof("Start ehco with version:%s", constant.Version)

	var rs *relay.Server
	if cfg.NeedStartRelayServer() {
		metrics.EhcoAlive.Set(metrics.EhcoAliveStateRunning)
		s, err := relay.NewServer(cfg)
		if err != nil {
			cliLogger.Fatalf("NewRelayServer meet err=%s", err.Error())
		}
		rs = s
		go func() {
			cliLogger.Fatalf("StartRelayServer meet err=%s", rs.Start(mainCtx))
		}()
	}

	if cfg.NeedStartWebServer() {
		webS, err := web.NewServer(cfg, rs, rs.Cmgr)
		if err != nil {
			cliLogger.Fatalf("NewWebServer meet err=%s", err.Error())
		}
		go func() {
			cliLogger.Fatalf("StartWebServer meet err=%s", webS.Start())
		}()
	}

	if cfg.NeedStartXrayServer() {
		xrayS := xray.NewXrayServer(cfg)
		if err := xrayS.Setup(); err != nil {
			cliLogger.Fatalf("Setup XrayServer meet err=%v", err)
		}
		if err := xrayS.Start(mainCtx); err != nil {
			cliLogger.Fatalf("Start XrayServer meet err=%v", err)
		}
	}
}
