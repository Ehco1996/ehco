package xray

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Ehco1996/ehco/internal/config"
	"github.com/Ehco1996/ehco/internal/tls"
	"github.com/Ehco1996/ehco/internal/web"
	xlog "github.com/xtls/xray-core/common/log"
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
			// Inject TLS certs for standard TLS inbounds if not provided by config
			if inbound.StreamSetting.TLSSettings == nil {
				inbound.StreamSetting.TLSSettings = &conf.TLSConfig{}
			}
			if len(inbound.StreamSetting.TLSSettings.Certs) == 0 {
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

	reloadMu  sync.Mutex
	runningMu sync.RWMutex
	// runningInbounds is the last set of proxy inbounds handed to xray,
	// keyed by tag ("listen,port"). Written by Setup (under runningMu),
	// read by the reload check and the admin runtime view.
	runningInbounds map[string]string
	// drift is set when the newest config differs from what is running
	// (a reload is pending or has failed), and cleared on a successful
	// reload. Surfaces on /overview so a stuck port is visible without
	// digging through logs.
	drift atomic.Bool
	// configSync is the last upstream config fetch; events is this
	// process's bounded lifecycle log. All in-memory by design.
	configSync syncStatus
	events     eventRing
	// counters is the flat since-start registry; the same instance is
	// handed to the outbound handler and the user pool so every
	// increment lands in one place.
	counters *counters
}

func NewXrayServer(cfg *config.Config) *XrayServer {
	return &XrayServer{
		l:               zap.L().Named("xray"),
		cfg:             cfg,
		tracker:         newConnTracker(),
		runningInbounds: make(map[string]string),
		counters:        &counters{},
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

	// Replace xray-core's default log handler (stdout/stderr) with a zap
	// bridge so xray's lines flow through the same WebSocket fan-out used by
	// ehco's own zap output. Must run after core.New, which installs xray's
	// default handler from cfg.Log; Reload re-runs Setup so this re-applies.
	xlog.RegisterHandler(newZapBridgeHandler(zap.L().Named("xray-core")))

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
		xs.up = NewUserPool(xs.cfg.SyncTrafficEndPoint, proxyTags, xs.counters)
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
	if err := om.AddHandler(context.Background(), newMeteredOutbound(xs.tracker, xs.up, xs.counters)); err != nil {
		return fmt.Errorf("register metered outbound: %w", err)
	}

	running := make(map[string]string)
	for _, inbound := range xs.cfg.XRayConfig.InboundConfigs {
		if InProxyTags(inbound.Tag) {
			listenStr := fmt.Sprintf("%s,%s", inbound.ListenOn.Address.String(), inbound.PortList.String())
			running[inbound.Tag] = listenStr
		}
	}
	xs.runningMu.Lock()
	xs.runningInbounds = running
	xs.runningMu.Unlock()

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
		xs.instance = nil
	}
	if xs.fallBack != nil {
		if err := xs.fallBack.Close(); err != nil {
			xs.l.Error("stop fallback server meet error", zap.Error(err))
		}
		xs.fallBack = nil
	}
	if xs.up != nil {
		xs.up.Stop()
		xs.up = nil
	}
}

func (xs *XrayServer) startInstance(ctx context.Context) error {
	if xs.instance == nil {
		return errors.New("xray instance is nil, call Setup first")
	}
	if err := xs.instance.Start(); err != nil {
		return err
	}
	if xs.fallBack != nil {
		go func() {
			if err := xs.fallBack.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				xs.l.Error("fallback server meet error", zap.Error(err))
			}
		}()
	}

	if xs.up != nil {
		if err := xs.up.Start(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (xs *XrayServer) Start(ctx context.Context) error {
	xs.l.Info("Start Xray Server now...")
	if xs.mainCtx == nil {
		xs.mainCtx = ctx
	}

	if err := xs.startInstance(ctx); err != nil {
		return err
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
					if loadErr := newCfg.LoadConfig(false); loadErr != nil {
						xs.configSync.record(loadErr)
						xs.counters.configFail.Add(1)
						xs.events.add("config_error", loadErr.Error())
						xs.l.Error("Reload Config meet error will retry in next loop", zap.Error(loadErr))
						continue
					}
					xs.configSync.record(nil)
					xs.counters.configOK.Add(1)
					needReload, err := xs.needReload(newCfg)
					if err != nil {
						xs.l.Error("check need reload meet error", zap.Error(err))
						continue
					}
					if needReload {
						xs.cfg = newCfg
						if err := xs.Reload(false); err != nil {
							xs.l.Error("Reload Xray Server meet error will retry in next loop", zap.Error(err))
							continue
						}
						xs.l.Info("Reload Xray Server success")
					}
				}
			}
		}()
	}
	return nil
}

func (xs *XrayServer) needReload(newCfg *config.Config) (bool, error) {
	if newCfg == nil || newCfg.XRayConfig == nil {
		return false, nil
	}
	xs.runningMu.RLock()
	running := make(map[string]string, len(xs.runningInbounds))
	for k, v := range xs.runningInbounds {
		running[k] = v
	}
	xs.runningMu.RUnlock()

	if len(running) == 0 {
		xs.markDrift("no running inbounds")
		return true, nil
	}

	newCfgM := make(map[string]string)
	for _, newInbound := range newCfg.XRayConfig.InboundConfigs {
		if !InProxyTags(newInbound.Tag) {
			continue
		}
		newListen := fmt.Sprintf("%s,%s", newInbound.ListenOn.Address.String(), newInbound.PortList.String())
		newCfgM[newInbound.Tag] = newListen
	}

	if len(running) != len(newCfgM) {
		xs.l.Info("inbound count changed, need restart instance",
			zap.Int("running", len(running)), zap.Int("new", len(newCfgM)))
		xs.markDrift(fmt.Sprintf("inbound count %d -> %d", len(running), len(newCfgM)))
		return true, nil
	}

	for tag, runListen := range running {
		newListen, ok := newCfgM[tag]
		if !ok {
			xs.l.Info("find inbound tag removed, need restart instance", zap.String("tag", tag))
			xs.markDrift("removed " + tag)
			return true, nil
		}
		if runListen != newListen {
			xs.l.Warn("find listener changed reload inbound now...",
				zap.String("old", runListen),
				zap.String("new", newListen),
				zap.String("tag", tag))
			xs.markDrift(fmt.Sprintf("%s %s -> %s", tag, runListen, newListen))
			return true, nil
		}
	}
	return false, nil
}

// markDrift flags the running config as out of sync. The lifecycle log
// gets one entry per episode: repeated ticks while a reload keeps
// failing add noise without new information (the reload events cover
// those).
func (xs *XrayServer) markDrift(detail string) {
	if !xs.drift.Swap(true) {
		xs.events.add("drift", detail)
	}
}

func (xs *XrayServer) Reload(force bool) error {
	err := xs.reload(force)
	// A successful reload just reset runningInbounds to the desired
	// config; a failure leaves whatever was running in place.
	xs.drift.Store(err != nil)
	if err != nil {
		xs.counters.reloadFail.Add(1)
		xs.events.add("reload_error", err.Error())
	} else {
		xs.counters.reloadOK.Add(1)
		reason := "config change"
		if force {
			reason = "manual"
		}
		xs.events.add("reload_ok", reason)
	}
	return err
}

func (xs *XrayServer) reload(force bool) error {
	xs.l.Warn("Reload Xray Server now...")
	xs.reloadMu.Lock()
	defer xs.reloadMu.Unlock()

	if force && xs.cfg.NeedSyncFromServer() {
		newCfg := config.NewConfig(xs.cfg.PATH)
		loadErr := newCfg.LoadConfig(true)
		xs.configSync.record(loadErr)
		if loadErr != nil {
			xs.counters.configFail.Add(1)
			xs.l.Error("Reload Xray Server load config error", zap.Error(loadErr))
			return loadErr
		}
		xs.counters.configOK.Add(1)
		xs.cfg = newCfg
	}

	xs.Stop()
	if err := xs.Setup(); err != nil {
		return err
	}
	if err := xs.startInstance(xs.mainCtx); err != nil {
		return err
	}
	return nil
}
