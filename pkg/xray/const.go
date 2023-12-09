package xray

const (
	XrayAPITag         = "api"
	XraySSProxyTag     = "ss_proxy"
	XrayTrojanProxyTag = "trojan_proxy"
	XrayVmessProxyTag  = "vmess_proxy"
	XrayVlessProxyTag  = "vless_proxy"
	XraySSRProxyTag    = "ssr_proxy"

	SyncTime = 60

	ProtocolSS     = "ss"
	ProtocolTrojan = "trojan"
)

func InProxyTags(tag string) bool {
	return tag == XraySSProxyTag || tag == XrayTrojanProxyTag ||
		tag == XrayVmessProxyTag || tag == XrayVlessProxyTag ||
		tag == XraySSRProxyTag
}
