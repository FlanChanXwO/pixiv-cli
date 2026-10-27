// Package fanboxapi 提供 FANBOX 的认证请求传输、Cloudflare challenge 恢复和资源 locator
// 缓存。
//
// 本包不承载内容语义：路由与响应映射由 sdk/fanbox 持有。
//
// 包的组成：[Client] 由 [New] 构造，持有 cookie、传输和 solver 状态；[Options] 与
// [FlareSolverrOptions] 是配置，[Locator] 是已校验的媒体地址，[MediaRequest] 约束
// 资源读取可用的条件头；protocol.go、identity.go、resource.go 提供校验与身份解析。
package fanboxapi

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
)

const (
	// WebBaseURL 指定 FANBOX 网页入口。
	WebBaseURL = "https://www.fanbox.cc/"
	// APIBaseURL 指定 FANBOX API 入口；sdk/fanbox 用它拼接 api.go 中的路由常量。
	APIBaseURL = "https://api.fanbox.cc/"
	// PostListPageLimit 指定 FANBOX 列表路由使用的 upstream 页大小参数，续页仍由服务端
	// 返回的 URL 决定。
	PostListPageLimit = 10
	// userAgent 保存既有的 Firefox 请求头基线；TLS 指纹在 transport.go 中独立配置。
	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:148.0) Gecko/20100101 Firefox/148.0"
)

// ErrForbidden 等 sentinel 是传输层的分类错误；公开 SDK 层依据它们做进一步分类。
var (
	// ErrForbidden 表示 FANBOX 拒绝访问（非 Cloudflare challenge）。
	ErrForbidden = errors.New("fanbox: forbidden")
	// ErrChallenge 表示 403 响应体或 header 表明触发了 Cloudflare challenge。
	ErrChallenge = errors.New("fanbox: challenge required")
	// ErrNotAuthenticated 表示会话缺失或过期（401）。
	ErrNotAuthenticated = errors.New("fanbox: not authenticated")
	// ErrInvalidOption 表示显式 FANBOX 连接选项格式错误；公开 SDK 把它映射为
	// InvalidArgument，且不回显原值。
	ErrInvalidOption = errors.New("fanbox: invalid option")
	// ErrMalformedResponse 表示 API JSON 或身份 metadata 无法解析。
	ErrMalformedResponse = errors.New("fanbox: malformed response")
)

// Locator 保存已通过媒体策略校验的 URL 及其附件文件名。
type Locator struct {
	URL      string
	Filename string
}

// Client 持有 FANBOX 传输、会话 Cookie、solver 状态和本次进程内的 locator 缓存。
//
// 格式化与日志输出不会带出 Cookie；只关闭由本 client 创建的 HTTP 连接。
type Client struct {
	httpClient   *http.Client
	ownsHTTP     bool
	userAgent    string
	cookie       string
	flareSolverr *FlareSolverrOptions
	log          *slog.Logger

	solverMu     sync.Mutex
	solverState  *solverState
	solverActive *solverCall
	locatorMu    sync.RWMutex
	locators     map[string]Locator
	identityMu   sync.Mutex
	userID       int64
}

// FlareSolverrOptions 保存可选 challenge 恢复路径使用的两个独立地址；solver 状态只留在
// 单个 [Client] 上。
type FlareSolverrOptions struct {
	URL      string
	ProxyURL string
}

// Options 提供 FANBOX 原生传输与 solver 的显式配置；FlareSolverr 为 nil 时不启用 solver。
type Options struct {
	HTTPClient   *http.Client
	ProxyURL     string
	UserAgent    string
	FlareSolverr *FlareSolverrOptions
	// Logger 记录请求结果和挑战恢复决策；nil 时不输出。
	Logger *slog.Logger
}

// New 构造一次传输运行时，不发起网络请求；sessionValue 只接受 FANBOXSESSID 的值。
func New(sessionValue string, options Options) (*Client, error) {
	agent, err := validateUserAgent(options.UserAgent)
	if err != nil {
		return nil, err
	}
	proxyURL, err := validateProxyURL(options.ProxyURL)
	if err != nil {
		return nil, err
	}
	solver, err := normalizeFlareSolverrOptions(options.FlareSolverr)
	if err != nil {
		return nil, err
	}
	client := options.HTTPClient
	ownsHTTP := client == nil
	if ownsHTTP {
		transport, err := newBrowserTransport(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("create FANBOX TLS transport: %w", err)
		}
		client = &http.Client{Transport: transport}
	} else if client.Transport == nil {
		return nil, errors.New("FANBOX injected HTTP client requires an explicit transport")
	}
	cookie, err := sessionCookie(sessionValue)
	if err != nil {
		return nil, err
	}
	return &Client{
		httpClient:   client,
		ownsHTTP:     ownsHTTP,
		userAgent:    agent,
		cookie:       cookie,
		flareSolverr: solver,
		log:          cmp.Or(options.Logger, slog.New(slog.DiscardHandler)),
		locators:     make(map[string]Locator),
	}, nil
}

// String 只输出类型摘要，调试格式化不会打印 Cookie。
func (*Client) String() string { return "fanboxapi.Client{}" }

// GoString 同 [Client.String]，用于 %#v。
func (*Client) GoString() string { return "fanboxapi.Client{}" }

// CloseIdleConnections 只关闭由本 client 创建的 HTTP 连接，注入的 client 由调用方管理。
func (c *Client) CloseIdleConnections() {
	if c.ownsHTTP {
		c.httpClient.CloseIdleConnections()
	}
}

// GetJSON 向 APIBaseURL 下的 path 发起一次带会话的请求，并把 JSON 解码到 target。
func (c *Client) GetJSON(ctx context.Context, path string, query url.Values, target any) error {
	endpoint := APIBaseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	return c.GetJSONURL(ctx, endpoint, target)
}

// CacheLocator 记录 ref 对应的最新媒体 locator，供后续同 ref 的资源读取使用。
func (c *Client) CacheLocator(ref string, locator Locator) {
	c.locatorMu.Lock()
	c.locators[ref] = locator
	c.locatorMu.Unlock()
}

// CachedLocator 返回本次 client 生命周期内为 ref 记录的 locator。
func (c *Client) CachedLocator(ref string) (Locator, bool) {
	c.locatorMu.RLock()
	locator, ok := c.locators[ref]
	c.locatorMu.RUnlock()
	return locator, ok
}

// VerifiedUserID 返回已认证的 FANBOX user id，每个 client 只验证一次。id 非秘密，
// 可以绑定进按身份生成的 cursor；session cookie 不出传输层。
func (c *Client) VerifiedUserID(ctx context.Context) (int64, error) {
	c.identityMu.Lock()
	defer c.identityMu.Unlock()
	if c.userID > 0 {
		return c.userID, nil
	}
	identity, err := c.CurrentUser(ctx)
	if err != nil {
		return 0, err
	}
	c.userID = identity.UserID
	return c.userID, nil
}
