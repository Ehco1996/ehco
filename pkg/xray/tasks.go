package xray

import (
	"context"
	"log"
	"net/http"
	"time"

	proxy "github.com/xtls/xray-core/app/proxyman/command"
	stats "github.com/xtls/xray-core/app/stats/command"
	"google.golang.org/grpc"
)

var API_ENDPOINT string
var GRPC_ENDPOINT string
var PROTOCOL_ENDPOINT string

const (
	VMESS  string = "vmess"
	TROJAN string = "trojan"
)

type UserConfig struct {
	UserId int    `json:"user_id"`
	Email  string `json:"email"`
	Level  uint32 `json:"level"`
	Enable bool   `json:"enable"`
	VmessConfig
	TrojanConfig
}
type VmessConfig struct {
	UUID    string `json:"uuid"`
	AlterId uint32 `json:"alter_id"`
}
type TrojanConfig struct {
	Password string `json:"password"`
}
type UserTraffic struct {
	UserId          int   `json:"user_id"`
	DownloadTraffic int64 `json:"dt"`
	UploadTraffic   int64 `json:"ut"`
}

type syncReq struct {
	UserTraffics []*UserTraffic `json:"user_traffics"`
}

type syncResp struct {
	Configs  []*UserConfig
	Tag      string `json:"tag"`
	Protocol string `json:"protocol"`
}

func SyncTask(up *UserPool) {

	// Connect to v2ray rpc
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, GRPC_ENDPOINT, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Printf("[WARNING]: GRPC连接失败,请检查V2ray是否运行并开放对应grpc端口 当前GRPC地址: %v 错误信息: %v", GRPC_ENDPOINT, err.Error())
		return
	} else {
		defer conn.Close()
	}



	resp := syncResp{}
	err = getJson(httpClient, API_ENDPOINT, &resp)
	if err != nil {
		log.Printf("[WARNING]: API连接失败,请检查API地址 当前地址: %v 错误信息:%v", API_ENDPOINT, err.Error())
		return
	}

	// init or update user config
	initOrUpdateUser(up, proxymanClient, &resp)

	// sync user traffic
	syncUserTrafficToServer(up, statClient, httpClient)
}

func initOrUpdateUser(up *UserPool, c proxy.HandlerServiceClient, sr *syncResp) {
	// Enable line numbers in logging
	log.Println("[INFO] Call initOrUpdateUser")

	syncUserMap := make(map[string]bool)

	for _, cfg := range sr.Configs {
		syncUserMap[cfg.Email] = true
		user, _ := up.GetUserByEmail(cfg.Email)
		if user == nil {
			// New User
			newUser, err := up.CreateUser(cfg.UserId, sr.Protocol, cfg.Email, cfg.UUID, cfg.Password, cfg.Level, cfg.AlterId, cfg.Enable)
			if err != nil {
				log.Fatalln(err)
			}
			if newUser.Enable {
				AddInboundUser(c, sr.Tag, sr.Protocol, newUser)
			}
		} else {
			// Old User
			if user.Enable != cfg.Enable {
				// update enable status
				user.setEnable(cfg.Enable)
			}
			// change user uuid
			if user.UUID != cfg.UUID && user.UUID != "" {
				log.Printf("[INFO] user: %s 更换了uuid old: %s new: %s", user.Email, user.UUID, cfg.UUID)
				RemoveInboundUser(c, sr.Tag, user)
				user.setUUID(cfg.UUID)
				AddInboundUser(c, sr.Tag, sr.Protocol, user)
			}
			// remove not enable user
			if !user.Enable && user.running {
				// Close Not Enable user
				RemoveInboundUser(c, sr.Tag, user)
			}
			// start not runing user
			if user.Enable && !user.running {
				// Start Not Running user
				AddInboundUser(c, sr.Tag, sr.Protocol, user)
			}
			if user.Password != cfg.Password && user.Password != "" {
				log.Printf("[INFO] user: %s 更换了password old: %s new: %s", user.Email, user.Password, cfg.Password)
				RemoveInboundUser(c, sr.Tag, user)
				user.setPassword(cfg.Password)
				AddInboundUser(c, sr.Tag, sr.Protocol, user)
			}
		}
	}

	// remote user not in server
	for _, user := range up.GetAllUsers() {
		if _, ok := syncUserMap[user.Email]; !ok {
			RemoveInboundUser(c, sr.Tag, user)
			up.RemoveUserByEmail(user.Email)
		}
	}
}

