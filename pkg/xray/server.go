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
	"github.com/xtls/xray-core/features/inbound"
	"github.com/xtls/xray-core/features/outbound"
	"github.com/xtls/xray-core/infra/conf"
	_ "github.com/xtls/xray-core/main/distro/all" // register all features
	"github.com/xtls/xray-core/proxy/trojan"
	"go.uber.org/zap"
)

// stripUnused removes the api/stats/policy/outbound configuration from the
// xray-core conf.Config. We no longer use xray's gRPC api or its stats counters
// (replaced by the in-process inbound.Manager + meteredOutbound), so leaving
// these in place would just bind ports and accumulate dead counters.
//
// The inbound tagged "api" (a dokodemo-door listener that serves the gRPC
// commander) is also dropped so the configured port is freed.
func stripUnused(cfg *conf.Config) {
	cfg.API = nil
	cfg.Stats = nil
	cfg.Policy = nil
	cfg.OutboundConfigs = nil

	if len(cfg.InboundConfigs) > 0 {
		filtered := cfg.InboundConfigs[:0]
		for _, in := range cfg.InboundConfigs {
			if in.Tag == XrayAPITag {
				continue
			}
			filtered = append(filtered, in)
		}
		cfg.InboundConfigs = filtered
	}
}

func buildXrayInstanceCfg(cfg *conf.Config) (*core.Config, error) {
	for _, inbound := range cfg.InboundConfigs {
		if inbound.Tag == XrayTrojanProxyTag || inbound.Tag == XrayVmessProxyTag || inbound.Tag == XrayVlessProxyTag {
			// Skip TLS cert injection for Reality — it uses its own key management
			if inbound.StreamSetting != nil && inbound.StreamSetting.Security == "reality" {
				if inbound.StreamSetting.SocketSettings != nil {
					inbound.StreamSetting.SocketSettings.TcpMptcp = true
				} else {
					inbound.StreamSetting.SocketSettings = &conf.SocketConfig{
						TcpMptcp: true,
					}
				}
				continue
			}
			// Inject TLS certs for standard TLS inbounds
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
	stripUnused(cfg)
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
	tracker  *connTracker
	fallBack *http.Server
	instance *core.Instance

	mainCtx context.Context
}

func NewXrayServer(cfg *config.Config) *XrayServer {
	return &XrayServer{
		l:       zap.L().Named("xray"),
		cfg:     cfg,
		tracker: newConnTracker(),
	}
}

// Tracker exposes the active connection registry so the admin API can list/kill conns.
func (xs *XrayServer) Tracker() *connTracker { return xs.tracker }

// UserPool exposes the in-process user pool. May be nil when sync is disabled.
func (xs *XrayServer) UserPool() *UserPool { return xs.up }

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
		var proxyTags []string
		for _, inbound := range xs.cfg.XRayConfig.InboundConfigs {
			if InProxyTags(inbound.Tag) {
				proxyTags = append(proxyTags, inbound.Tag)
			}
		}
		if len(proxyTags) == 0 {
			return errors.New("can't find proxy tag in config")
		}
		xs.up = NewUserPool(xs.cfg.SyncTrafficEndPoint, xs.cfg.GetMetricURL(), proxyTags)
		xs.up.SetConnTracker(xs.tracker)

		im, ok := instance.GetFeature(inbound.ManagerType()).(inbound.Manager)
		if !ok || im == nil {
			return errors.New("xray inbound manager feature missing")
		}
		xs.up.SetInboundManager(im)
	}

	// Register our metered outbound as the default. We stripped all outbound
	// configs from cfg, so no other handler exists yet — AddHandler with an
	// empty tag becomes the default handler used by the dispatcher.
	om, ok := instance.GetFeature(outbound.ManagerType()).(outbound.Manager)
	if !ok || om == nil {
		return errors.New("xray outbound manager feature missing")
	}
	if err := om.AddHandler(context.Background(), newMeteredOutbound(xs.tracker, xs.up)); err != nil {
		return fmt.Errorf("register metered outbound: %w", err)
	}
	return nil
}

func (xs *XrayServer) Stop() {
	xs.l.Warn("Stop Xray Server now...")
	if xs.tracker != nil {
		killed := xs.tracker.KillAll()
		if killed > 0 {
			xs.l.Sugar().Infof("Killed %d active conns on stop", killed)
		}
	}
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
							xs.l.Info("Reload Xray Server Once success")
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
	// xs.cfg is shared with relay's reloader; LoadConfig nils c.XRayConfig
	// before the (possibly slow) HTTP fetch + Unmarshal. Snapshot the pointer
	// so we don't deref a freshly-nil'd field mid-evaluation, and skip the
	// tick if either side isn't ready — next tick re-evaluates after the
	// concurrent reload settles.
	var oldXC, newXC *conf.Config
	if xs.cfg != nil {
		oldXC = xs.cfg.XRayConfig
	}
	if newCfg != nil {
		newXC = newCfg.XRayConfig
	}
	if oldXC == nil || newXC == nil {
		xs.l.Warn("skip needReload: xray config not ready (concurrent reload in progress?)")
		return false, nil
	}
	oldCfgM := make(map[string]conf.InboundDetourConfig)
	for _, inbound := range oldXC.InboundConfigs {
		if InProxyTags(inbound.Tag) {
			oldCfgM[inbound.Tag] = inbound
		}
	}
	for _, newInbound := range newXC.InboundConfigs {
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
