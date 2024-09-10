package conf

type HandshakePayload struct {
	FinalAddr    string        `json:"final_addr"`
	RemoteChains []ChianRemote `json:"remote_chains"`
}

func BuildHandshakePayload(cfg *Config, finalAddr string) *HandshakePayload {
	return &HandshakePayload{FinalAddr: finalAddr, RemoteChains: cfg.RemoteChains}
}

func (p *HandshakePayload) removeLocalChain(nodeLabel string) {
	for i, remote := range p.RemoteChains {
		if remote.NodeLabel == nodeLabel {
			p.RemoteChains = append(p.RemoteChains[:i], p.RemoteChains[i+1:]...)
		}
	}
}
