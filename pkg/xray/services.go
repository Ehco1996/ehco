package xray

import (
	"context"
	"strings"

	"github.com/Ehco1996/ehco/internal/logger"
	proxy "github.com/xtls/xray-core/app/proxyman/command"
	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/proxy/shadowsocks"
)

func getEmailAndTrafficType(input string) (string, string) {
	s := strings.Split(input, ">>>")
	return s[1], s[len(s)-1]
}

// AddInboundUser add user to inbound by tag
func AddInboundUser(ctx context.Context, c proxy.HandlerServiceClient, tag string, user *User) error {
	resp, err := c.AlterInbound(ctx, &proxy.AlterInboundRequest{
		Tag: tag,
		Operation: serial.ToTypedMessage(&proxy.AddUserOperation{
			User: &protocol.User{
				Level: uint32(user.Level),
				Email: user.GetEmail(),
				Account: serial.ToTypedMessage(&shadowsocks.Account{
					CipherType: shadowsocks.CipherType(user.Method),
					Password:   user.Password}),
			},
		}),
	})
	if err != nil {
		return err
	}
	logger.Infof("[xray] Add User: %v To Server Tag: %v resp: %s", user.ID, tag, resp)
	user.running = true
	return nil
}

//RemoveInboundUser remove user from inbound by tag
func RemoveInboundUser(ctx context.Context, c proxy.HandlerServiceClient, tag string, user *User) error {
	resp, err := c.AlterInbound(ctx, &proxy.AlterInboundRequest{
		Tag: tag,
		Operation: serial.ToTypedMessage(&proxy.RemoveUserOperation{
			Email: user.GetEmail(),
		}),
	})
	if err != nil {
		return err

	}
	logger.Infof("[xray] Remove User: %v From Server Tag: %v resp: %s", user.ID, tag, resp)
	user.running = false
	return nil
}
