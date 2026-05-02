package xray

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Ehco1996/ehco/pkg/bytes"
	myhttp "github.com/Ehco1996/ehco/pkg/http"
	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/features/inbound"
	"github.com/xtls/xray-core/proxy"
	"github.com/xtls/xray-core/proxy/shadowsocks_2022"
	"github.com/xtls/xray-core/proxy/trojan"
	"github.com/xtls/xray-core/proxy/vless"
	"go.uber.org/zap"
)

type User struct {
	running bool

	ID       int    `json:"user_id"`
	Method   string `json:"method"`
	Password string `json:"password"`
	Flow     string `json:"flow"`

	Level  int  `json:"level"`
	Enable bool `json:"enable"`

	// Updated atomically by the metered outbound. Snapshotted (and reset)
	// every SyncTime seconds by syncTrafficToServer.
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

func (u *User) AddUploadTraffic(n int64) {
	atomic.AddInt64(&u.UploadTraffic, n)
}

func (u *User) AddDownloadTraffic(n int64) {
	atomic.AddInt64(&u.DownloadTraffic, n)
}

// snapshotAndReset returns the accumulated up/down byte counts and resets
// them in a single atomic swap, so concurrent increments aren't lost.
func (u *User) snapshotAndReset() (up, down int64) {
	up = atomic.SwapInt64(&u.UploadTraffic, 0)
	down = atomic.SwapInt64(&u.DownloadTraffic, 0)
	return
}

func (u *User) UpdateFromServer(serverSideUser *User) {
	u.Method = serverSideUser.Method
	u.Enable = serverSideUser.Enable
	u.Password = serverSideUser.Password
	u.Flow = serverSideUser.Flow
}

func (u *User) Equal(new *User) bool {
	return u.Method == new.Method && u.Enable == new.Enable && u.Password == new.Password && u.Flow == new.Flow
}

// toMemoryUser builds an xray-core MemoryUser describing this user's account
// for the configured protocol. Returns nil for unknown protocols.
func (u *User) toMemoryUser() *protocol.MemoryUser {
	var account *serial.TypedMessage
	switch u.Protocol {
	case ProtocolTrojan:
		account = serial.ToTypedMessage(&trojan.Account{Password: u.Password})
	case ProtocolSS:
		memoryAccount := &shadowsocks_2022.MemoryAccount{Key: u.Password}
		account = serial.ToTypedMessage(memoryAccount.ToProto())
	case ProtocolVless:
		account = serial.ToTypedMessage(&vless.Account{Id: u.Password, Flow: u.Flow})
	default:
		zap.S().DPanicf("unknown protocol %s", u.Protocol)
		return nil
	}
	pu := &protocol.User{Level: uint32(u.Level), Email: u.GetEmail(), Account: account}
	mu, err := pu.ToMemoryUser()
	if err != nil {
		zap.S().Named("xray").Errorf("build memory user for %d failed: %v", u.ID, err)
		return nil
	}
	return mu
}

type UserPool struct {
	l *zap.Logger
	sync.RWMutex
	// map key : ID
	users map[int]*User

	im inbound.Manager
	br *bandwidthRecorder

	proxyTags       []string
	cancel          context.CancelFunc
	remoteConfigURL string
}

func NewUserPool(remoteConfigURL, metricURL string, proxyTags []string) *UserPool {
	up := &UserPool{
		l:               zap.L().Named("user_pool"),
		users:           make(map[int]*User),
		proxyTags:       proxyTags,
		remoteConfigURL: remoteConfigURL,
	}
	if metricURL != "" {
		up.br = NewBandwidthRecorder(metricURL)
	}
	return up
}

// SetInboundManager wires the in-process xray inbound.Manager that the pool
// uses to add/remove users on each protocol's inbound.
func (up *UserPool) SetInboundManager(im inbound.Manager) {
	up.im = im
}

func (up *UserPool) CreateUser(userId, level int, password, method, protocol, flow string, enable bool) *User {
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
		Flow:     flow,
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

// userManagerFor returns the in-process proxy.UserManager for the given inbound tag.
func (up *UserPool) userManagerFor(ctx context.Context, tag string) (proxy.UserManager, error) {
	if up.im == nil {
		return nil, errors.New("inbound manager not set")
	}
	handler, err := up.im.GetHandler(ctx, tag)
	if err != nil {
		return nil, fmt.Errorf("get inbound handler %q: %w", tag, err)
	}
	gi, ok := handler.(proxy.GetInbound)
	if !ok {
		return nil, fmt.Errorf("inbound %q does not expose proxy.GetInbound", tag)
	}
	um, ok := gi.GetInbound().(proxy.UserManager)
	if !ok {
		return nil, fmt.Errorf("inbound %q does not implement proxy.UserManager", tag)
	}
	return um, nil
}

func (up *UserPool) addInboundUser(ctx context.Context, tag string, user *User) error {
	um, err := up.userManagerFor(ctx, tag)
	if err != nil {
		return err
	}
	mu := user.toMemoryUser()
	if mu == nil {
		return fmt.Errorf("build memory user for %d", user.ID)
	}
	if err := um.AddUser(ctx, mu); err != nil {
		// xray returns "already exists" when re-adding; treat as benign.
		if isAlreadyExists(err) {
			up.l.Sugar().Infof("user %s already on inbound %s", user.GetEmail(), tag)
		} else {
			return fmt.Errorf("add user %s to inbound %s: %w", user.GetEmail(), tag, err)
		}
	}
	user.running = true
	up.l.Sugar().Infof("Add user %s to inbound %s", user.GetEmail(), tag)
	return nil
}

func (up *UserPool) removeInboundUser(ctx context.Context, tag string, user *User) error {
	um, err := up.userManagerFor(ctx, tag)
	if err != nil {
		return err
	}
	if err := um.RemoveUser(ctx, user.GetEmail()); err != nil {
		if isNotFound(err) {
			up.l.Sugar().Warnf("user %s not on inbound %s", user.GetEmail(), tag)
		} else {
			return fmt.Errorf("remove user %s from inbound %s: %w", user.GetEmail(), tag, err)
		}
	}
	user.running = false
	up.l.Sugar().Infof("Remove user %s from inbound %s", user.GetEmail(), tag)
	return nil
}

func isAlreadyExists(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "already exists")
}

func isNotFound(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "not found")
}

func (up *UserPool) syncTrafficToServer(ctx context.Context) error {
	tfs := make([]*UserTraffic, 0)
	for _, user := range up.GetAllUsers() {
		up_, down := user.snapshotAndReset()
		if up_ == 0 && down == 0 {
			continue
		}
		up.l.Sugar().Infof("User %d traffic up=%d down=%d", user.ID, up_, down)
		tfs = append(tfs, &UserTraffic{
			ID:              user.ID,
			UploadTraffic:   up_,
			DownloadTraffic: down,
			IPList:          []string{},
			TcpCount:        0,
		})
	}

	req := &SyncTrafficReq{Data: tfs}
	if up.br != nil {
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
			"Total Increment Per User :", bytes.PrettyByteSize(float64(req.GetTotalTraffic())),
		)
	}
	// TODO: traffic in `tfs` was atomically reset; if the upstream POST
	// fails (after retries), this batch is lost. Persist unsent batches
	// locally and replay them on the next tick.
	if err := myhttp.PostJSONWithRetry(up.remoteConfigURL, req); err != nil {
		return err
	}
	up.l.Sugar().Infof("syncTrafficToServer ONLINE USER COUNT: %d", len(tfs))
	return nil
}

func (up *UserPool) syncUserConfigsFromServer(ctx context.Context, proxyTag string) error {
	resp := SyncUserConfigsResp{}
	if err := myhttp.GetJSONWithRetry(up.remoteConfigURL, &resp); err != nil {
		return err
	}
	userM := make(map[int]struct{})
	for _, newUser := range resp.Users {
		oldUser, found := up.GetUser(newUser.ID)
		if !found {
			created := up.CreateUser(
				newUser.ID, newUser.Level, newUser.Password, newUser.Method, newUser.Protocol, newUser.Flow, newUser.Enable)
			if created.Enable {
				if err := up.addInboundUser(ctx, proxyTag, created); err != nil {
					return err
				}
			}
		} else {
			if !oldUser.Equal(newUser) {
				oldUser.UpdateFromServer(newUser)
				if oldUser.running {
					if err := up.removeInboundUser(ctx, proxyTag, oldUser); err != nil {
						return err
					}
				}
			}
			if oldUser.Enable && !oldUser.running {
				if err := up.addInboundUser(ctx, proxyTag, oldUser); err != nil {
					return err
				}
			}
		}
		userM[newUser.ID] = struct{}{}
	}
	for _, user := range up.GetAllUsers() {
		if _, ok := userM[user.ID]; !ok {
			if err := up.removeInboundUser(ctx, proxyTag, user); err != nil {
				return err
			}
			up.RemoveUser(user.ID)
		}
	}
	return nil
}

func (up *UserPool) Start(ctx context.Context) error {
	if up.im == nil {
		return errors.New("UserPool: inbound manager not set; call SetInboundManager before Start")
	}

	syncOnce := func() error {
		for _, tag := range up.proxyTags {
			if err := up.syncUserConfigsFromServer(ctx, tag); err != nil {
				up.l.Sugar().Errorf("Sync User Configs From Server Error: %v", err)
				return err
			}
		}
		// Traffic is pool-wide, so push once after all tags are reconciled.
		if err := up.syncTrafficToServer(ctx); err != nil {
			up.l.Sugar().Errorf("Sync Traffic To Server Error: %v", err)
			return err
		}
		return nil
	}
	// Tolerate a failed initial sync: starting up with an empty user pool is
	// preferable to a crash-loop when the upstream is briefly unreachable.
	if err := syncOnce(); err != nil {
		up.l.Sugar().Errorf("Initial sync failed, will retry on next tick in %ds: %v", SyncTime, err)
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
					up.l.Error("sync failed, will retry on next tick", zap.Error(err))
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
