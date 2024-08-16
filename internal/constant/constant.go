package constant

import "time"

type RelayType string

var (
	// allow change in test
	// TODO Set to Relay Config
	ReadTimeOut = 5 * time.Second
	IdleTimeOut = 30 * time.Second

	Version     = "1.1.5-dev"
	GitBranch   string
	GitRevision string
	BuildTime   string
	StartTime   = time.Now().Local()
)

const (
	DialTimeOut = 3 * time.Second

	SniffTimeOut = 300 * time.Millisecond

	// todo,support config in relay config
	BUFFER_POOL_SIZE = 1024      // support 512 connections
	BUFFER_SIZE      = 40 * 1024 // 40KB ,the maximum packet size of shadowsocks is about 16 KiB so this is enough
	UDPBufSize       = 1500      // use default max mtu 1500
)

// relay type
const (
	// direct relay
	RelayTypeRaw RelayType = "raw"
	// ws relay
	RelayTypeWS  RelayType = "ws"
	RelayTypeWSS RelayType = "wss"
)
