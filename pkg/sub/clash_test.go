package sub

import (
	"testing"

	"github.com/Ehco1996/ehco/pkg/log"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

var l *zap.Logger

func init() {
	log.InitGlobalLogger("debug")
	l = zap.L().Named("clash_test")
}

func TestNewClashConfig(t *testing.T) {
	// todo add more proxy types
	buf := []byte(`
proxies:
  - name: ss
    type: ss
    server: proxy1.example.com
    port: 1080
    password: password
    cipher: aes-128-gcm
    udp: true
  - name: trojan
    type: trojan
    server: proxy2.example.com
    port: 443
    password: password
    skip-cert-verify: true
`)
	cs, err := NewClashSub(buf)
	assert.NoError(t, err, "NewConfig should not return an error")
	assert.NotNil(t, cs, "Config should not be nil")
	expectedProxyCount := 2
	assert.Equal(t, expectedProxyCount, len(cs.raw.Proxies), "Proxy count should match")

	yamlBuf, err := cs.ToClashConfigYaml()
	assert.NoError(t, err, "ToClashConfigYaml should not return an error")
	assert.NotNil(t, yamlBuf, "yamlBuf should not be nil")
	println(string(yamlBuf))
}
