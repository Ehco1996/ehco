package transporter

import (
	"context"
	"crypto/tls"

	"github.com/Ehco1996/ehco/internal/relay/conf"
	mytls "github.com/Ehco1996/ehco/internal/tls"
)

var (
	_ RelayClient = &WssClient{}
	_ RelayServer = &WssServer{}
)

type WssClient struct {
	*WsClient
}

func newWssClient(cfg *conf.Config) (*WssClient, error) {
	wc, err := newWsClient(cfg)
	if err != nil {
		return nil, err
	}
	// insert tls config
	wc.dialer.TLSConfig = mytls.DefaultTLSConfig
	wc.dialer.TLSConfig.InsecureSkipVerify = true
	return &WssClient{WsClient: wc}, nil
}

type WssServer struct {
	*WsServer
}

func newWssServer(bs *BaseRelayServer) (*WssServer, error) {
	wsServer, err := newWsServer(bs)
	if err != nil {
		return nil, err
	}
	return &WssServer{WsServer: wsServer}, nil
}

func (s *WssServer) ListenAndServe(ctx context.Context) error {
	listener, err := NewTCPListener(ctx, s.cfg)
	if err != nil {
		return err
	}
	tlsCfg := mytls.DefaultTLSConfig
	tlsCfg.InsecureSkipVerify = true
	tlsListener := tls.NewListener(listener, mytls.DefaultTLSConfig)
	return s.httpServer.Serve(tlsListener)
}
