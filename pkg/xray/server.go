package xray

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Ehco1996/ehco/internal/config"
	"github.com/Ehco1996/ehco/internal/tls"
	"github.com/Ehco1996/ehco/internal/web"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf"
	_ "github.com/xtls/xray-core/main/distro/all" // register all features
	"github.com/xtls/xray-core/proxy/trojan"
	"go.uber.org/zap"
)

func buildXrayInstanceCfg(cfg *conf.Config) (*core.Config, error) {
	for _, inbound := range cfg.InboundConfigs {
		// add tls certs for trojan
		if inbound.Tag == XrayTrojanProxyTag || inbound.Tag == XrayVmessProxyTag || inbound.Tag == XrayVlessProxyTag {
			if err := tls.InitTlsCfg(); err != nil {
				return nil, err
			}
			tlsConfigs := []*conf.TLSCertConfig{
				{
					CertStr: []string{string(tls.DefaultTLSConfigCertBytes)},
					KeyStr:  []string{string(tls.DefaultTLSConfigKeyBytes)},
				},
			}
			inbound.StreamSetting.TLSSettings.Certs = tlsConfigs
		}
	}
	coreCfg, err := cfg.Build()
	if err != nil {
		return nil, err
	}
	return coreCfg, nil
}

type XrayServer struct {
	l   *zap.Logger
	cfg *config.Config

	up       *UserPool
	fallBack *http.Server
	instance *core.Instance
}

func NewXrayServer(cfg *config.Config) (*XrayServer, error) {
	xs := &XrayServer{l: zap.L().Named("xray"), cfg: cfg}
	coreCfg, err := buildXrayInstanceCfg(cfg.XRayConfig)
	if err != nil {
		return nil, err
	}
	for _, inbound := range coreCfg.Inbound {
		if inbound.Tag == XrayTrojanProxyTag {
			ins, err := inbound.ProxySettings.GetInstance()
			if err != nil {
				return nil, err
			}
			// // add fake fallback http server
			s := ins.(*trojan.ServerConfig)
			if len(s.Fallbacks) > 0 {
				dest := s.Fallbacks[0].Dest
				zap.L().Info("start fallback server for trojan at", zap.String("dest", dest))
				mux := http.NewServeMux()
				mux.HandleFunc("/", web.MakeIndexF())
				xs.fallBack = &http.Server{Addr: dest, Handler: mux}
			}
		}
	}
	instance, err := core.New(coreCfg)
	if err != nil {
		return nil, err
	}
	xs.instance = instance

	if xs.cfg.SyncTrafficEndPoint != "" {
		// find api port and server, hard code api Tag to `api`
		var grpcEndPoint string
		var proxyTags []string
		for _, inbound := range xs.cfg.XRayConfig.InboundConfigs {
			if inbound.Tag == XrayAPITag {
				grpcEndPoint = fmt.Sprintf("%s:%d", inbound.ListenOn.String(), inbound.PortList.Range[0].From)
			}
			if InProxyTags(inbound.Tag) {
				proxyTags = append(proxyTags, inbound.Tag)
			}
		}
		if grpcEndPoint == "" {
			return nil, errors.New("can't find api port in config")
		}
		if len(proxyTags) == 0 {
			return nil, errors.New("can't find proxy tag in config")
		}
		xs.up = NewUserPool(grpcEndPoint, xs.cfg.SyncTrafficEndPoint, xs.cfg.GetMetricURL(), proxyTags)
	}
	return xs, nil
}

//nolint:errcheck
func (xs *XrayServer) Stop() {
	if xs.up != nil {
		xs.up.Stop()
	}
	if xs.instance != nil {
		xs.instance.Close()
	}
	if xs.fallBack != nil {
		xs.fallBack.Close()
	}
}

func (xs *XrayServer) Start(ctx context.Context) error {
	if err := xs.instance.Start(); err != nil {
		return err
	}
	xs.l.Info("Start Xray Server")
	if xs.up != nil {
		if err := xs.up.Start(ctx); err != nil {
			return err
		}
		xs.l.Info("Start Xray User Pool")
	}

	if xs.cfg.ReloadInterval > 0 {
		go func() {
			ticker := time.NewTicker(time.Second * time.Duration(xs.cfg.ReloadInterval))
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if err := xs.ReloadProxyInbound(); err != nil {
						xs.l.Error("Reload Config meet error", zap.Error(err))
					}
				}
			}
		}()
	}
	return nil
}

func (xs *XrayServer) ReloadProxyInbound() error {
	// todo: implement reload proxy inbound
	// 1. close old proxy inbound when not in new config
	// 2. add new proxy inbound when not in old config
	// 3. update proxy inbound when port changed
	oldCfgM := make(map[string]*conf.InboundDetourConfig)
	for _, inbound := range xs.cfg.XRayConfig.InboundConfigs {
		if InProxyTags(inbound.Tag) {
			oldCfgM[inbound.Tag] = &inbound
		}
	}

	newCfg := config.NewConfig(xs.cfg.PATH)
	if err := newCfg.LoadConfig(); err != nil {
		xs.l.Error("Reload Config meet error", zap.Error(err))
		return err
	}
	for _, inbound := range newCfg.XRayConfig.InboundConfigs {
		if !InProxyTags(inbound.Tag) {
			continue
		}
		oldCfg, ok := oldCfgM[inbound.Tag]
		if !ok {
			xs.l.Info("find new inbound config", zap.String("tag", inbound.Tag))
			continue
		}
		if oldCfg.PortList.Build().String() != inbound.PortList.Build().String() {
			xs.l.Warn("find listen port changed, need restart xray server",
				zap.String("old", oldCfg.PortList.Build().String()),
				zap.String("new", inbound.PortList.Build().String()),
				zap.String("tag", inbound.Tag))
		}
	}
	return nil
}
