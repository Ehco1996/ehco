package sub

type ClashConfig struct {
	Proxies []Proxies `yaml:"proxies"`
}

type Proxies struct {
	// basic fields
	Name     string `yaml:"name"`
	Type     string `yaml:"type"`
	Server   string `yaml:"server"`
	Port     int    `yaml:"port"`
	Password string `yaml:"password,omitempty"`
	UDP      bool   `yaml:"udp,omitempty"`

	// for shadowsocks todo(support opts)
	Cipher string `yaml:"cipher,omitempty"`

	// for trojan todo(support opts)
	ALPN           []string `yaml:"alpn,omitempty"`
	SkipCertVerify bool     `yaml:"skip-cert-verify,omitempty"`
	SNI            string   `yaml:"sni,omitempty"`
	Network        string   `yaml:"network,omitempty"`

	// for socks5 todo(support opts)
	UserName string `yaml:"username,omitempty"`
	TLS      bool   `yaml:"tls,omitempty"`

	// for vmess todo(support opts)
	UUID       string `yaml:"uuid,omitempty"`
	AlterID    int    `yaml:"alterId,omitempty"`
	ServerName string `yaml:"servername,omitempty"`
}
