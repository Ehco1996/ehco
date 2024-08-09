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
			if inbound.StreamSetting.SocketSettings != nil {
				inbound.StreamSetting.SocketSettings.TcpMptcp = true
			} else {
				inbound.StreamSetting.SocketSettings = &conf.SocketConfig{
					TcpMptcp: true,
				}
			}
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

	mainCtx context.Context
}

func NewXrayServer(cfg *config.Config) *XrayServer {
	return &XrayServer{l: zap.L().Named("xray"), cfg: cfg}
}

func (xs *XrayServer) Setup() error {
	coreCfg, err := buildXrayInstanceCfg(xs.cfg.XRayConfig)
	if err != nil {
		return err
	}
	for _, inbound := range coreCfg.Inbound {
		if inbound.Tag == XrayTrojanProxyTag {
			ins, err := inbound.ProxySettings.GetInstance()
			if err != nil {
				return err
			}
			// add fake fallback http server
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
		return err
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
			return errors.New("can't find api port in config")
		}
		if len(proxyTags) == 0 {
			return errors.New("can't find proxy tag in config")
		}
		xs.up = NewUserPool(grpcEndPoint, xs.cfg.SyncTrafficEndPoint, xs.cfg.GetMetricURL(), proxyTags)
	}
	return nil
}

func (xs *XrayServer) Stop() {
	xs.l.Warn("Stop Xray Server now...")
	if xs.instance != nil {
		if err := xs.instance.Close(); err != nil {
			xs.l.Error("stop instance meet error", zap.Error(err))
		}
	}
	if xs.fallBack != nil {
		if err := xs.fallBack.Close(); err != nil {
			xs.l.Error("stop fallback server meet error", zap.Error(err))
		}
	}
	if xs.up != nil {
		xs.up.Stop()
	}
}

func (xs *XrayServer) Start(ctx context.Context) error {
	xs.l.Info("Start Xray Server now...")
	if err := xs.instance.Start(); err != nil {
		return err
	}
	if xs.fallBack != nil {
		go func() {
			if err := xs.fallBack.ListenAndServe(); err != nil {
				xs.l.Error("fallback server meet error", zap.Error(err))
			}
		}()
	}

	if xs.up != nil {
		if err := xs.up.Start(ctx); err != nil {
			return err
		}
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
					newCfg := config.NewConfig(xs.cfg.PATH)
					if err := newCfg.LoadConfig(false); err != nil {
						// TODO refine
						xs.l.Error("Reload Config meet error will retry in next loop", zap.Error(err))
						continue
					}
					if needReload, err := xs.needReload(newCfg); err != nil {
						xs.l.Error("check need reload meet error", zap.Error(err))
					} else {
						if needReload {
							xs.cfg = newCfg
							if err := xs.Reload(); err != nil {
								xs.l.Error("Reload Xray Server meet error", zap.Error(err))
							}
							xs.l.Warn("Reload Xray Server success exit watcher ...")
							return
						}
					}
				}
			}
		}()
	}
	if xs.mainCtx == nil {
		xs.mainCtx = ctx
	}
	return nil
}

func (xs *XrayServer) needReload(newCfg *config.Config) (bool, error) {
	oldCfgM := make(map[string]conf.InboundDetourConfig)
	for _, inbound := range xs.cfg.XRayConfig.InboundConfigs {
		if InProxyTags(inbound.Tag) {
			oldCfgM[inbound.Tag] = inbound
		}
	}
	for _, newInbound := range newCfg.XRayConfig.InboundConfigs {
		if !InProxyTags(newInbound.Tag) {
			continue
		}
		oldInbound, ok := oldCfgM[newInbound.Tag]
		if !ok {
			xs.l.Info("find new inbound config, need restart instance", zap.String("tag", newInbound.Tag))
			return true, nil
		}
		oldListen := fmt.Sprintf("%s,%s", oldInbound.ListenOn.Address.String(), oldInbound.PortList.Build().String())
		newListen := fmt.Sprintf("%s,%s", newInbound.ListenOn.Address.String(), newInbound.PortList.Build().String())
		xs.l.Debug("check listen port",
			zap.String("old", oldListen), zap.String("new", newListen), zap.String("tag", newInbound.Tag))
		if oldListen != newListen {
			xs.l.Warn("find listener changed reload inbound now...",
				zap.String("old", oldListen),
				zap.String("new", newListen),
				zap.String("tag", newInbound.Tag))
			return true, nil
		}
	}
	return false, nil
}

func (xs *XrayServer) Reload() error {
	xs.l.Warn("Reload Xray Server now...")
	xs.Stop()
	if err := xs.Setup(); err != nil {
		return err
	}
	if err := xs.Start(xs.mainCtx); err != nil {
		return err
	}
	return nil
}
