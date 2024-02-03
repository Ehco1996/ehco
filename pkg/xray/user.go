package xray

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/Ehco1996/ehco/pkg/bytes"
	proxy "github.com/xtls/xray-core/app/proxyman/command"
	stats "github.com/xtls/xray-core/app/stats/command"
	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/proxy/shadowsocks"
	"github.com/xtls/xray-core/proxy/trojan"
	"go.uber.org/zap"
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
	Data              []*UserTraffic `json:"data"`
	UploadBandwidth   int64          `json:"upload_bandwidth"`
	DownloadBandwidth int64          `json:"download_bandwidth"`
}

func (s *SyncTrafficReq) GetTotalTraffic() int64 {
	var total int64
	for _, u := range s.Data {
		total += u.UploadTraffic + u.DownloadTraffic
	}
	return total
}

type SyncUserConfigsResp struct {
	Users []*User `json:"users"`
}

// NOTE we use user id as email
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
	var account *serial.TypedMessage
	switch u.Protocol {
	case ProtocolTrojan:
		account = serial.ToTypedMessage(&trojan.Account{Password: u.Password})
	case ProtocolSS:
		account = serial.ToTypedMessage(&shadowsocks.Account{CipherType: mappingCipher(u.Method), Password: u.Password})
	default:
		zap.S().DPanicf("unknown protocol %s", u.Protocol)
		return nil
	}
	return &protocol.User{Level: uint32(u.Level), Email: u.GetEmail(), Account: account}
}

type UserPool struct {
	l *zap.Logger
	sync.RWMutex
	// map key : ID
	users map[int]*User

	httpClient  *http.Client
	proxyClient proxy.HandlerServiceClient
	statsClient stats.StatsServiceClient

	br *bandwidthRecorder

	proxyTags       []string
	cancel          context.CancelFunc
	grpcEndPoint    string
	remoteConfigURL string
}

func NewUserPool(grpcEndPoint, remoteConfigURL, metricURL string, proxyTags []string) *UserPool {
	up := &UserPool{
		l:               zap.L().Named("user_pool"),
		users:           make(map[int]*User),
		proxyTags:       proxyTags,
		grpcEndPoint:    grpcEndPoint,
		remoteConfigURL: remoteConfigURL,
	}
	if metricURL != "" {
		up.br = NewBandwidthRecorder(metricURL)
	}
	return up
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

func (up *UserPool) syncTrafficToServer(ctx context.Context, proxyTag string) error {
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
			up.l.Sugar().Warnf(
				"user in xray not found in user pool this user maybe out of traffic, user id: %d, leak traffic: %d",
				userID, stat.Value)
			fakeUser := &User{ID: userID}
			if err := RemoveInboundUser(ctx, up.proxyClient, proxyTag, fakeUser); err != nil {
				up.l.Warn("tring remove leak user failed, user id: %d err: %s",
					zap.Int("user_id", userID), zap.Error(err))
			}
			continue
		}
		// Note v2ray 只会统计 inbound 的流量，所以这里乘 2 以补偿 outbound 的流量
		switch trafficType {
		case "uplink":
			user.UploadTraffic = stat.Value * 2
		case "downlink":
			user.DownloadTraffic = stat.Value * 2
		}
	}

	tfs := make([]*UserTraffic, 0, len(up.users))
	for _, user := range up.GetAllUsers() {
		tf := user.DownloadTraffic + user.UploadTraffic
		if tf > 0 {
			up.l.Sugar().Infof("User: %v Now Used Total Traffic: %v", user.ID, tf)
			tfs = append(tfs, user.GenTraffic())
			user.ResetTraffic()
		}
	}
	req := &SyncTrafficReq{Data: tfs}
	if up.br != nil {
		// record bandwidth
		uploadIncr, downloadIncr, err := up.br.RecordOnce(ctx)
		if err != nil {
			return err
		}

		ub := up.br.GetUploadBandwidth()
		req.UploadBandwidth = int64(ub)
		db := up.br.GetDownloadBandwidth()
		req.DownloadBandwidth = int64(db)
		up.l.Sugar().Debug(
			"Upload Bandwidth :", bytes.PrettyByteSize(ub),
			"Download Bandwidth :", bytes.PrettyByteSize(db),
			"Total Bandwidth :", bytes.PrettyByteSize(ub+db),
			"Total Increment By BR", bytes.PrettyByteSize(uploadIncr+downloadIncr),
			"Total Increment By Xray :", bytes.PrettyByteSize(float64(req.GetTotalTraffic())),
		)
	}
	if err := postJson(up.httpClient, up.remoteConfigURL, req); err != nil {
		return err
	}
	up.l.Sugar().Infof("Call syncTrafficToServer ONLINE USER COUNT: %d", len(tfs))
	return nil
}

func (up *UserPool) syncUserConfigsFromServer(ctx context.Context, proxyTag string) error {
	resp := SyncUserConfigsResp{}
	if err := getJson(up.httpClient, up.remoteConfigURL, &resp); err != nil {
		return err
	}
	userM := make(map[int]struct{})
	for _, newUser := range resp.Users {
		oldUser, found := up.GetUser(newUser.ID)
		if !found {
			newUser := up.CreateUser(
				newUser.ID, newUser.Level, newUser.Password, newUser.Method, newUser.Protocol, newUser.Enable)
			if newUser.Enable {
				if err := AddInboundUser(ctx, up.proxyClient, proxyTag, newUser); err != nil {
					return err
				}
			}
		} else {
			// update user configs
			if !oldUser.Equal(newUser) {
				oldUser.UpdateFromServer(newUser)
				if oldUser.running {
					if err := RemoveInboundUser(ctx, up.proxyClient, proxyTag, oldUser); err != nil {
						return err
					}
				}
			}
			if oldUser.Enable && !oldUser.running {
				if err := AddInboundUser(ctx, up.proxyClient, proxyTag, oldUser); err != nil {
					return err
				}
			}
		}
		userM[newUser.ID] = struct{}{}
	}
	// remove user not in server
	for _, user := range up.GetAllUsers() {
		if _, ok := userM[user.ID]; !ok {
			if err := RemoveInboundUser(ctx, up.proxyClient, proxyTag, user); err != nil {
				return err
			}
			up.RemoveUser(user.ID)
		}
	}
	return nil
}

func (up *UserPool) Start(ctx context.Context) error {
	conn, err := grpc.DialContext(
		context.Background(), up.grpcEndPoint, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return err
	}
	up.proxyClient = proxy.NewHandlerServiceClient(conn)
	up.statsClient = stats.NewStatsServiceClient(conn)
	up.httpClient = &http.Client{Timeout: time.Second * 10}

	syncOnce := func() error {
		for _, tag := range up.proxyTags {
			if err := up.syncUserConfigsFromServer(ctx, tag); err != nil {
				up.l.Sugar().Errorf("Sync User Configs From Server Error: %v", err)
				return err
			}
			if err := up.syncTrafficToServer(ctx, tag); err != nil {
				up.l.Sugar().Errorf("Sync Traffic From Server Error: %v", err)
				return err
			}
		}
		return nil
	}
	if err := syncOnce(); err != nil {
		return err
	}

	ctx2, cancel := context.WithCancel(ctx)
	up.cancel = cancel
	go func() {
		ticker := time.NewTicker(time.Second * SyncTime)
		for {
			select {
			case <-ctx2.Done():
				return
			case <-ticker.C:
				if err := syncOnce(); err != nil {
					up.l.Error("Sync User Configs From Server Error: %v", zap.Error(err))
				}
			}
		}
	}()
	return nil
}

func (up *UserPool) Stop() {
	if up.cancel != nil {
		up.cancel()
	}
}
