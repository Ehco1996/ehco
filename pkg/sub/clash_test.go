package sub

import (
	"testing"

	"github.com/Ehco1996/ehco/pkg/log"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

var (
	l *zap.Logger

	configBuf = []byte(`
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
)

func init() {
	log.InitGlobalLogger("debug")
	l = zap.L().Named("clash_test")
}

func TestNewClashConfig(t *testing.T) {
	// todo add more proxy types

	cs, err := NewClashSub(configBuf, "test", "")
	assert.NoError(t, err, "NewConfig should not return an error")
	assert.NotNil(t, cs, "Config should not be nil")
	expectedProxyCount := 2
	assert.Equal(t, expectedProxyCount, len(*cs.cCfg.Proxies), "Proxy count should match")

	yamlBuf, err := cs.ToClashConfigYaml()
	assert.NoError(t, err, "ToClashConfigYaml should not return an error")
	assert.NotNil(t, yamlBuf, "yamlBuf should not be nil")
	l.Info("yamlBuf", zap.String("yamlBuf", string(yamlBuf)))
}

func TestToRelayConfigs(t *testing.T) {
	cs, err := NewClashSub(configBuf, "test", "")
	assert.NoError(t, err, "NewConfig should not return an error")
	assert.NotNil(t, cs, "Config should not be nil")

	relayConfigs, err := cs.ToRelayConfigs("localhost")
	assert.NoError(t, err, "ToRelayConfigs should not return an error")
	assert.NotNil(t, relayConfigs, "relayConfigs should not be nil")
	expectedRelayCount := 4 // 2 proxy + 2 load balance
	assert.Equal(t, expectedRelayCount, len(relayConfigs), "Relay count should match")
	l.Info("relayConfigs", zap.Any("relayConfigs", relayConfigs))
}
