package xray

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/Ehco1996/ehco/internal/config"
	"github.com/Ehco1996/ehco/internal/tls"
	"github.com/Ehco1996/ehco/internal/web"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf"
	_ "github.com/xtls/xray-core/main/distro/all" // register all features
	"github.com/xtls/xray-core/proxy/trojan"
)

const (
	XrayAPITag         = "api"
	XraySSProxyTag     = "ss_proxy"
	XrayTrojanProxyTag = "trojan_proxy"
	XrayVmessProxyTag  = "vmess_proxy"
	XrayVlessProxyTag  = "vless_proxy"
	XraySSRProxyTag    = "ssr_proxy"
	
	SyncTime = 60
)

func StartXrayServer(ctx context.Context, cfg *config.Config) (*core.Instance, error) {
	initXrayLogger()
	for _, inbound := range cfg.XRayConfig.InboundConfigs {
		// add tls certs for trojan
		if inbound.Tag == XrayTrojanProxyTag {
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
		if inbound.Tag == XrayVmessProxyTag {
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
		if inbound.Tag == XrayVlessProxyTag {
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

	coreCfg, err := cfg.XRayConfig.Build()
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
			for idx := range s.Fallbacks {
				dest := s.Fallbacks[idx].Dest
				go func() {
					l.Infof("start fallback server for trojan at %s", dest)
					mux := http.NewServeMux()
					mux.HandleFunc("/", web.MakeIndexF(l))
					s := &http.Server{Addr: dest, Handler: mux}
					l.Fatal(s.ListenAndServe())
				}()
			}
		}
	}

	server, err := core.New(coreCfg)
	if err != nil {
		return nil, err
	}

	if err := server.Start(); err != nil {
		return nil, err
	}
	return server, nil
}

func StartSyncTask(ctx context.Context, cfg *config.Config) error {
	initXrayLogger()
	// find api port and server, hard code api Tag to `api`
	var grpcEndPoint string
	var proxyTag string
	for _, inbound := range cfg.XRayConfig.InboundConfigs {
		if inbound.Tag == XrayAPITag {
			grpcEndPoint = fmt.Sprintf("%s:%d", inbound.ListenOn.String(), inbound.PortList.Range[0].From)
		}
		if inbound.Tag == XraySSProxyTag {
			proxyTag = XraySSProxyTag
		}
		if inbound.Tag == XrayTrojanProxyTag {
			proxyTag = XrayTrojanProxyTag
		}
	}

	if grpcEndPoint == "" {
		return errors.New("[xray] can't find api port in config")
	}
	if proxyTag == "" {
		return errors.New("[xray] can't find proxy tag in config")
	}

	l.Infof("api port: %s, proxy tag: %s", grpcEndPoint, proxyTag)

	up, err := NewUserPool(ctx, grpcEndPoint)
	if err != nil {
		return err
	}
	up.StartSyncUserTask(ctx, cfg.SyncTrafficEndPoint, proxyTag)
	return nil
}
