package session

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/account"
	"github.com/FlanChanXwO/pixiv-cli/internal/config"
	"github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/secretfile"
)

// PixivSettings 保存一次请求翻译得到的 SDK 选项与只读会话缓存键。
// 代理覆盖、节奏与语言都由组合根解析，本包不读取配置或 flag。
type PixivSettings struct {
	Options pixiv.Options
	Network string
}

// ReadSnapshot 是一次只读 SDK 会话及其失效通知。
type ReadSnapshot struct {
	Lease       *lifecycle.Lease[*pixiv.Client]
	Version     string
	Expires     time.Time
	Invalidated <-chan struct{}
	Invalidate  func(error)
}

// LocalAccount 是本地账号的公开摘要；凭据始终留在账号存储中。
type LocalAccount struct {
	UserID         int64
	Username       string
	HasCredentials bool
}

// LocalAccounts 保存本地账号列表与 CLI 显式设置的默认账号。
type LocalAccounts struct {
	Accounts      []LocalAccount
	DefaultUserID int64
}

// Local 装配 CLI 与 MCP 共用的本地账号文件、Pixiv 账号池和 FANBOX 会话。
//
// 账号存储与各子系统都在第一次使用时才打开，help、version 不会提前创建它们；
// 读取会话登记到构造时传入的 [lifecycle.CloseState]。
type Local struct {
	closeState     *lifecycle.CloseState
	loadPixiv      func(ProxyRequest) (PixivSettings, error)
	loadFanbox     func() (fanbox.Options, error)
	loadPoolConfig func() (PoolConfig, error)

	accountsMu sync.Mutex
	accounts   *Accounts
	login      LoginService

	pixivOnce sync.Once
	pixiv     *pixivPorts
	pixivErr  error

	fanboxAccountsMu sync.Mutex
	fanboxAccounts   *FanboxAccounts

	fanboxOnce sync.Once
	fanbox     *Fanbox
	fanboxErr  error
}

type pixivPorts struct {
	store  *account.Store
	reads  *account.ReadSessions
	facade *Pixiv
}

// NewLocal 构造按需打开的本地账号装配；三个 loader 由组合根提供。
func NewLocal(
	closeState *lifecycle.CloseState,
	loadPixiv func(ProxyRequest) (PixivSettings, error),
	loadFanbox func() (fanbox.Options, error),
	loadPoolConfig func() (PoolConfig, error),
) *Local {
	return &Local{closeState: closeState, loadPixiv: loadPixiv, loadFanbox: loadFanbox, loadPoolConfig: loadPoolConfig}
}

func (l *Local) openAccounts() (*account.Store, error) {
	directory, err := config.AppDataDir()
	if err != nil {
		return nil, fmt.Errorf("determine home directory: %w", err)
	}
	store, err := account.Open(context.Background(), directory)
	if err != nil {
		return nil, err
	}
	return store, nil
}

// PixivAccounts 延迟打开 Pixiv 账号服务与登录服务；打开失败不缓存，后续调用可以重试。
func (l *Local) PixivAccounts() (*Accounts, LoginService, error) {
	l.accountsMu.Lock()
	defer l.accountsMu.Unlock()
	if l.accounts == nil {
		store, err := l.openAccounts()
		if err != nil {
			return nil, LoginService{}, err
		}
		l.accounts = &Accounts{store: store, defaults: config.Store{Files: secretfile.ConfigFiles{}}}
		l.login = LoginService{Pixiv: l.accounts}
	}
	return l.accounts, l.login, nil
}

// CurrentUser 只读取当前选中的账号摘要；不创建 Pixiv client，也不轮换凭据。
func (l *Local) CurrentUser(ctx context.Context) (*AccountSummary, error) {
	service, _, err := l.PixivAccounts()
	if err != nil {
		return nil, err
	}
	return service.CurrentUser(ctx)
}

// FanboxAccounts 打开一个 FANBOX 账号服务；每次调用都按文件版本打开存储。
func (l *Local) FanboxAccounts() (*FanboxAccounts, error) {
	store, err := l.openAccounts()
	if err != nil {
		return nil, err
	}
	return &FanboxAccounts{store: store, defaults: config.Store{Files: secretfile.ConfigFiles{}}, loadOptions: l.loadFanbox}, nil
}

// Fanbox 延迟构造命令组共用的 FANBOX 会话。
func (l *Local) Fanbox() (*Fanbox, error) {
	l.fanboxOnce.Do(func() {
		accounts, err := l.fanboxService()
		if err != nil {
			l.fanboxErr = err
			return
		}
		l.fanbox = NewFanbox(accounts)
	})
	return l.fanbox, l.fanboxErr
}

// NewFanbox 返回执行内共享账号服务的 FANBOX 会话，供 MCP 每次打开租约使用；
// 账号服务在本次执行内复用，不按请求累积。
func (l *Local) NewFanbox() (*Fanbox, error) {
	accounts, err := l.fanboxService()
	if err != nil {
		return nil, err
	}
	return NewFanbox(accounts), nil
}

func (l *Local) fanboxService() (*FanboxAccounts, error) {
	l.fanboxAccountsMu.Lock()
	defer l.fanboxAccountsMu.Unlock()
	if l.fanboxAccounts == nil {
		service, err := l.FanboxAccounts()
		if err != nil {
			return nil, err
		}
		l.fanboxAccounts = service
	}
	return l.fanboxAccounts, nil
}

func (l *Local) ports() (*pixivPorts, error) {
	l.pixivOnce.Do(func() {
		store, err := l.openAccounts()
		if err != nil {
			l.pixivErr = err
			return
		}
		service := &Accounts{store: store, defaults: config.Store{Files: secretfile.ConfigFiles{}}}
		gate := account.NewGate()
		reads := account.NewReadSessions(store, gate)
		l.closeState.Add(reads.Close)
		l.pixiv = &pixivPorts{
			store:  store,
			reads:  reads,
			facade: NewPixiv(service, gate, l.loadPoolConfig),
		}
	})
	return l.pixiv, l.pixivErr
}

// Execute 在账号池的重放边界内使用一次 Pixiv client。
func (l *Local) Execute(ctx context.Context, request ProxyRequest, callback func(context.Context, *pixiv.Client) (bool, error)) error {
	ports, err := l.ports()
	if err != nil {
		return err
	}
	settings, err := l.loadPixiv(request)
	if err != nil {
		return err
	}
	return ports.facade.Use(ctx, Request{Selection: selectionFor(request.UserID), Options: settings.Options}, callback)
}

// OpenLease 打开一个显式账号的 Pixiv client 租约。
func (l *Local) OpenLease(ctx context.Context, request ProxyRequest) (*lifecycle.Lease[*pixiv.Client], error) {
	ports, err := l.ports()
	if err != nil {
		return nil, err
	}
	settings, err := l.loadPixiv(request)
	if err != nil {
		return nil, err
	}
	return ports.facade.Open(ctx, Request{Selection: selectionFor(request.UserID), Options: settings.Options})
}

// Read 复用账号读取快照执行一次读取，并在失败时丢弃对应 generation。
func (l *Local) Read(ctx context.Context, request ProxyRequest, callback func(context.Context, *pixiv.Client) (bool, error)) error {
	snapshot, err := l.ReadSession(ctx, request)
	if err != nil {
		return err
	}
	defer snapshot.Lease.Close()
	_, err = callback(ctx, snapshot.Lease.Value())
	snapshot.Invalidate(err)
	return err
}

// ReadSession 复用账号读取快照，并按请求选择代理覆写与请求节奏。
func (l *Local) ReadSession(ctx context.Context, request ProxyRequest) (ReadSnapshot, error) {
	ports, err := l.ports()
	if err != nil {
		return ReadSnapshot{}, err
	}
	settings, err := l.loadPixiv(request)
	if err != nil {
		return ReadSnapshot{}, err
	}
	userID := request.UserID
	if userID == 0 {
		userID, err = ports.facade.accounts.selectedUserID(ctx)
		if err != nil {
			return ReadSnapshot{}, err
		}
	}
	snapshot, err := ports.reads.Open(ctx, userID, settings.Network, settings.Options)
	if err != nil {
		return ReadSnapshot{}, err
	}
	return ReadSnapshot{
		Lease:       lifecycle.NewLease(snapshot.Client, nil),
		Version:     fmt.Sprint(snapshot.Version),
		Expires:     snapshot.Expires,
		Invalidated: snapshot.Invalidated,
		Invalidate:  func(err error) { ports.reads.Invalidate(userID, snapshot.Version, err) },
	}, nil
}

// LocalAccounts 读取本地账号摘要与显式默认账号，不读取凭据。
func (l *Local) LocalAccounts(ctx context.Context) (LocalAccounts, error) {
	ports, err := l.ports()
	if err != nil {
		return LocalAccounts{}, err
	}
	records, err := ports.store.ListPixiv(ctx)
	if err != nil {
		return LocalAccounts{}, err
	}
	defaultID, explicit, err := (config.Store{Files: secretfile.ConfigFiles{}}).ReadPixivDefaultUserID()
	if err != nil {
		return LocalAccounts{}, err
	}
	local := LocalAccounts{Accounts: make([]LocalAccount, 0, len(records))}
	if explicit {
		local.DefaultUserID = defaultID
	}
	for _, record := range records {
		local.Accounts = append(local.Accounts, LocalAccount{UserID: record.UserID, Username: record.Username, HasCredentials: !record.Credential.Empty()})
	}
	return local, nil
}

// selectionFor 将外部请求的可选 UID 翻译为账号 owner 的显式选择值。
func selectionFor(userID int64) account.Selection {
	if userID != 0 {
		return account.Selection{Mode: account.SelectionFixed, UserID: userID}
	}
	return account.Selection{Mode: account.SelectionPool}
}
