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
		l.Error("Failed to Add User: %s To Server Tag: %s", user.GetEmail(), tag)
		return err
	}
	user.running = true
	l.Infof("Add User: %s To Server Tag: %s", user.GetEmail(), tag)
	return nil
}

// RemoveInboundUser remove user from inbound by tag
func RemoveInboundUser(ctx context.Context, c proxy.HandlerServiceClient, tag string, user *User) error {
	_, err := c.AlterInbound(ctx, &proxy.AlterInboundRequest{
		Tag: tag,
		Operation: serial.ToTypedMessage(&proxy.RemoveUserOperation{
			Email: user.GetEmail(),
		}),
	})

	// mute not found error
	if err != nil && strings.Contains(err.Error(), "not found") {
		l.Warnf("User Not Found  %s", user.GetEmail())
		err = nil
	}

	if err != nil {
		l.Error("Failed to Remove User: %s To Server", user.GetEmail())
		return err
	}
	user.running = false
	l.Infof("[xray] Remove User: %v From Server", user.ID)
	return nil
}
