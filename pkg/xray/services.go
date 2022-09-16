package xray

import (
	"context"
	"strings"

	proxy "github.com/xtls/xray-core/app/proxyman/command"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/proxy/shadowsocks"
)

func getEmailAndTrafficType(input string) (string, string) {
	s := strings.Split(input, ">>>")
	return s[1], s[len(s)-1]
}

func mappingCipher(in string) shadowsocks.CipherType {
	switch in {
	case "aes-128-gcm":
		return shadowsocks.CipherType_AES_128_GCM
	case "aes-256-gcm":
		return shadowsocks.CipherType_AES_256_GCM
	case "chacha20-ietf-poly1305":
		return shadowsocks.CipherType_CHACHA20_POLY1305
	}
	return shadowsocks.CipherType_UNKNOWN
}

// AddInboundUser add user to inbound by tag
func AddInboundUser(ctx context.Context, c proxy.HandlerServiceClient, tag string, user *User) error {
	_, err := c.AlterInbound(ctx, &proxy.AlterInboundRequest{
		Tag: tag,
		Operation: serial.ToTypedMessage(
			&proxy.AddUserOperation{User: user.ToXrayUser()}),
	})
	if err != nil {
		return err
	}
	l.Infof("Add User: %s To Server Tag: %s", user.GetEmail(), tag)
	user.running = true
	return nil
}

//RemoveInboundUser remove user from inbound by tag
func RemoveInboundUser(ctx context.Context, c proxy.HandlerServiceClient, tag string, user *User) error {
	_, err := c.AlterInbound(ctx, &proxy.AlterInboundRequest{
		Tag: tag,
		Operation: serial.ToTypedMessage(&proxy.RemoveUserOperation{
			Email: user.GetEmail(),
		}),
	})
	if err != nil {
		return err

	}
	l.Infof("[xray] Remove User: %v From Server", user.ID)
	user.running = false
	return nil
}
