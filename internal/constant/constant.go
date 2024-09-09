package constant

import "time"

type RelayType string

var (
	Version     = "1.1.5"
	GitBranch   string
	GitRevision string
	BuildTime   string
	StartTime   = time.Now().Local()
)

const (
	DefaultDialTimeOut  = 3 * time.Second
	DefaultReadTimeOut  = 5 * time.Second
	DefaultIdleTimeOut  = 10 * time.Second
	DefaultSniffTimeOut = 300 * time.Millisecond

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
