package transporter

import (
	mytls "github.com/Ehco1996/ehco/internal/tls"
)

var (
	_ RelayClient = &WssClient{}
	_ RelayServer = &WssServer{}
)

type WssClient struct {
	*WsClient
}

func newWssClient(base *baseTransporter) (*WssClient, error) {
	wc, err := newWsClient(base)
	if err != nil {
		return nil, err
	}
	// insert tls config
	wc.dialer.TLSConfig = mytls.DefaultTLSConfig
	return &WssClient{WsClient: wc}, nil
}

type WssServer struct {
	*WsServer
}

func newWssServer(base *baseTransporter) (*WssServer, error) {
	wsServer, err := newWsServer(base)
	if err != nil {
		return nil, err
	}
	// insert tls config
	wsServer.httpServer.TLSConfig = mytls.DefaultTLSConfig
	return &WssServer{WsServer: wsServer}, nil
}
