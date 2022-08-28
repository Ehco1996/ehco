package xray

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	proxy "github.com/xtls/xray-core/app/proxyman/command"
	stats "github.com/xtls/xray-core/app/stats/command"
	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/proxy/shadowsocks"
	"github.com/xtls/xray-core/proxy/trojan"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type User struct {
	running bool

	ID       int    `json:"user_id"`
	Method   string `json:"method"`
	Password string `json:"password"`

	Level           int   `json:"level"`
	Enable          bool  `json:"enable"`
	UploadTraffic   int64 `json:"upload_traffic"`
	DownloadTraffic int64 `json:"download_traffic"`

	Protocol string `json:"protocol"`
}

type UserTraffic struct {
	ID              int      `json:"user_id"`
	UploadTraffic   int64    `json:"upload_traffic"`
	DownloadTraffic int64    `json:"download_traffic"`
	IPList          []string `json:"ip_list"`
	TcpCount        int64    `json:"tcp_conn_num"`
}

type SyncTrafficReq struct {
	Data []*UserTraffic `json:"data"`
}

type SyncUserConfigsResp struct {
	Users []*User `json:"users"`
}

// NOTE we user user id as email
func (u *User) GetEmail() string {
	return fmt.Sprintf("%d", u.ID)
}

func (u *User) ResetTraffic() {
	u.DownloadTraffic = 0
	u.UploadTraffic = 0
}

func (u *User) GenTraffic() *UserTraffic {
	return &UserTraffic{
		ID:              u.ID,
		UploadTraffic:   u.UploadTraffic,
		DownloadTraffic: u.DownloadTraffic,
		IPList:          []string{},
		TcpCount:        0,
	}
}

func (u *User) UpdateFromServer(serverSideUser *User) {
	u.Method = serverSideUser.Method
	u.Enable = serverSideUser.Enable
	u.Password = serverSideUser.Password
}

func (u *User) Equal(new *User) bool {
	return u.Method == new.Method && u.Enable == new.Enable && u.Password == new.Password
}

func (u *User) ToXrayUser() *protocol.User {
	var account proto.Message
	switch u.Protocol {
	case "trojan":
		account = &trojan.Account{Password: u.Password}
	default:
		account = &shadowsocks.Account{
			CipherType: mappingCipher(u.Method),
			Password:   u.Password}
	}
	xu := &protocol.User{
		Level:   uint32(u.Level),
		Email:   u.GetEmail(),
		Account: serial.ToTypedMessage(account),
	}
	return xu
}

// UserPool user pool
type UserPool struct {
	sync.RWMutex
	// map key : ID
	users map[int]*User

	httpClient  *http.Client
	proxyClient proxy.HandlerServiceClient
	statsClient stats.StatsServiceClient
}

// NewUserPool New UserPool
func NewUserPool(ctx context.Context, xrayEndPoint string) (*UserPool, error) {
	conn, err := grpc.DialContext(ctx, xrayEndPoint, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return nil, err
	}

	// Init Client
	proxyClient := proxy.NewHandlerServiceClient(conn)
	statsClient := stats.NewStatsServiceClient(conn)
	httpClient := http.Client{Timeout: 30 * time.Second}

	up := &UserPool{
		users: make(map[int]*User),

		httpClient:  &httpClient,
		proxyClient: proxyClient,
		statsClient: statsClient,
	}

	return up, nil
}

func (up *UserPool) CreateUser(userId, level int, password, method, protocol string, enable bool) *User {
	up.Lock()
	defer up.Unlock()
	u := &User{
		running:  false,
		ID:       userId,
		Password: password,
		Level:    level,
		Enable:   enable,
		Method:   method,
		Protocol: protocol,
	}
	up.users[u.ID] = u
	return u
}

func (up *UserPool) GetUser(id int) (*User, bool) {
	up.RLock()
	defer up.RUnlock()
	user, ok := up.users[id]
	return user, ok
}

func (up *UserPool) RemoveUser(id int) {
	up.Lock()
	defer up.Unlock()
	delete(up.users, id)
}

func (up *UserPool) GetAllUsers() []*User {
	up.RLock()
	defer up.RUnlock()

	users := make([]*User, 0, len(up.users))
	for _, user := range up.users {
		users = append(users, user)
	}
	return users
}

func (up *UserPool) syncTrafficToServer(ctx context.Context, endpoint, tag string) error {
	// sync traffic from xray server
	// V2ray的stats的统计模块设计的非常奇怪，具体规则如下
	// 上传流量："user>>>" + user.Email + ">>>traffic>>>uplink"
	// 下载流量："user>>>" + user.Email + ">>>traffic>>>downlink"
	resp, err := up.statsClient.QueryStats(ctx, &stats.QueryStatsRequest{Pattern: "user>>>", Reset_: true})
	if err != nil {
		return err
	}

	for _, stat := range resp.Stat {
		userIDStr, trafficType := getEmailAndTrafficType(stat.Name)
		userID, err := strconv.Atoi(userIDStr)
		if err != nil {
			return err
		}
		user, found := up.GetUser(userID)
		if !found {
			L.Warnf(
				"user in xray not found in user pool this user maybe out of traffic, user id: %d, leak traffic: %d",
				userID, stat.Value)
			fakeUser := &User{ID: userID}
			if err := RemoveInboundUser(ctx, up.proxyClient, tag, fakeUser); err != nil {
				L.Warnf(
					"tring remove leak user failed, user id: %d err: %s", userID, err.Error())
			}
			continue
		}
		switch trafficType {
		case "uplink":
			user.UploadTraffic = stat.Value
		case "downlink":
			user.DownloadTraffic = stat.Value
		}
	}

	tfs := make([]*UserTraffic, 0, len(up.users))
	for _, user := range up.GetAllUsers() {
		tf := user.DownloadTraffic + user.UploadTraffic
		if tf > 0 {
			L.Infof("User: %v Now Used Total Traffic: %v", user.ID, tf)
			tfs = append(tfs, user.GenTraffic())
			user.ResetTraffic()
		}
	}
	if err := postJson(up.httpClient, endpoint, &SyncTrafficReq{Data: tfs}); err != nil {
		return err
	}
	L.Infof("Call syncTrafficToServer ONLINE USER COUNT: %d", len(tfs))
	return nil
}

func (up *UserPool) syncUserConfigsFromServer(ctx context.Context, endpoint, tag string) error {
	resp := SyncUserConfigsResp{}
	if err := getJson(up.httpClient, endpoint, &resp); err != nil {
		return err
	}
	userM := make(map[int]struct{})
	for _, newUser := range resp.Users {
		oldUser, found := up.GetUser(newUser.ID)
		if !found {
			newUser := up.CreateUser(
				newUser.ID, newUser.Level, newUser.Password, newUser.Method, newUser.Protocol, newUser.Enable)
			if newUser.Enable {
				if err := AddInboundUser(ctx, up.proxyClient, tag, newUser); err != nil {
					return err
				}
			}
		} else {
			// update user configs
			if !oldUser.Equal(newUser) {
				if oldUser.running {
					if err := RemoveInboundUser(ctx, up.proxyClient, tag, oldUser); err != nil {
						return err
					}
				}
				oldUser.UpdateFromServer(newUser)
			}
			if oldUser.Enable && !oldUser.running {
				if err := AddInboundUser(ctx, up.proxyClient, tag, oldUser); err != nil {
					return err
				}
			}
		}
		userM[newUser.ID] = struct{}{}
	}
	// remove user not in server
	for _, user := range up.GetAllUsers() {
		if _, ok := userM[user.ID]; !ok {
			if err := RemoveInboundUser(ctx, up.proxyClient, tag, user); err != nil {
				return err
			}
			up.RemoveUser(user.ID)
		}
	}
	return nil
}

func (up *UserPool) StartSyncUserTask(ctx context.Context, endpoint, tag string) {
	L.Infof("Start Sync User Task")

	syncOnce := func() {
		if err := up.syncUserConfigsFromServer(ctx, endpoint, tag); err != nil {
			L.Errorf("Sync User Configs From Server Error: %v", err)
		}
		if err := up.syncTrafficToServer(ctx, endpoint, tag); err != nil {
			L.Errorf("Sync Traffic From Server Error: %v", err)
		}
	}
	syncOnce()
	ticker := time.NewTicker(time.Second * SyncTime)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			syncOnce()
		}
	}
}
