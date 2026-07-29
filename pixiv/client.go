// Package pixiv 提供可嵌入 Go 程序的 Pixiv 客户端与稳定模型。
package pixiv

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	internalpixiv "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/appapi"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/model"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
	internalresource "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/resource"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/webapi"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/credentials"
)

// clientOptions 是两个公开构造器的内部完整配置。它不直接暴露，避免调用方把
// 某个构造器不会读取的字段混在一起。
type clientOptions struct {
	// Logger 接收调用方显式注入的诊断 logger；为空时 SDK 严格静默，绝不使用 slog.Default。
	Logger *slog.Logger
	// HTTPClient 同时承载 App、Web、OAuth 与资源请求；为空时 SDK 创建一个
	// 无整请求固定超时的专用 client，请求生命周期由 context 控制。
	HTTPClient *http.Client
	// AppAPIBaseURL 覆盖 App API 地址，主要用于代理与测试。
	AppAPIBaseURL string
	// WebAPIBaseURL 覆盖 Pixiv Web 地址，主要用于页面补全与测试。
	WebAPIBaseURL string
	// AccessToken 是调用 App API 使用的 bearer token。
	AccessToken string
	// RefreshToken 仅供 OpenDefault 的本次快照显式选择；NewClient 不读取它。
	RefreshToken string
	// UserID 仅供 OpenDefault 从本地 auth store 选择账号。
	UserID int64
	// OAuthBaseURL 覆盖 OAuth token endpoint，主要用于测试。
	OAuthBaseURL string
	// AuthFilePath 和 ConfigFilePath 指定本地状态路径。NewClient 不会读取它们；
	// OpenDefault 在为空时使用现有默认路径。
	AuthFilePath   string
	ConfigFilePath string
	// WebFallbackEnabled 允许无 access token 时显式使用匿名 Web API。
	WebFallbackEnabled bool
	// ResourcePolicy 追加调用方显式信任的资源镜像；Pixiv 官方资源规则始终启用。
	ResourcePolicy ResourcePolicy
	// ResourceCachePath 非空时将 HTTP 缓存保存在此目录；为空时使用下载目录中的 .pixiv-cache。
	ResourceCachePath string
	// IgnoreEnvironmentRefreshToken 使 OpenDefault 只从显式 token 或本地 auth store
	// 选择凭据。零值保持 SDK 原有的环境变量优先级。
	IgnoreEnvironmentRefreshToken bool
	// DisableRetryAfterRetry 让调用方在首个有效 429 时接管调度。零值保持读取请求
	// 基于 Retry-After 的一次自动重试。
	DisableRetryAfterRetry bool
}

// NewClientOptions 配置显式 access-token 或匿名 Web API Client。它不读取本地
// auth/config 状态；需要本地默认账号时使用 OpenDefault 或 OpenDefaultWith。
type NewClientOptions struct {
	Logger                 *slog.Logger
	HTTPClient             *http.Client
	AppAPIBaseURL          string
	WebAPIBaseURL          string
	AccessToken            string
	WebFallbackEnabled     bool
	ResourcePolicy         ResourcePolicy
	ResourceCachePath      string
	DisableRetryAfterRetry bool
}

// OpenDefaultOptions 配置从本地 auth/config 与环境变量取得身份的 Client。
// AccessToken 和 WebFallbackEnabled 不属于这个类型：前者用于 NewClient，后者由
// 本地配置快照决定。
type OpenDefaultOptions struct {
	Logger                        *slog.Logger
	HTTPClient                    *http.Client
	AppAPIBaseURL                 string
	WebAPIBaseURL                 string
	OAuthBaseURL                  string
	AuthFilePath                  string
	ConfigFilePath                string
	ResourcePolicy                ResourcePolicy
	ResourceCachePath             string
	RefreshToken                  string
	UserID                        int64
	IgnoreEnvironmentRefreshToken bool
	DisableRetryAfterRetry        bool
}

// Client 组合 App API 主数据与显式 Web 补全能力。
type Client struct {
	app                *appapi.Client
	web                *webapi.Client
	authenticated      bool
	webFallbackEnabled bool
	resourcePolicy     resourcePolicy
	resource           *internalresource.Client
	// resourceDownloads 串行化同一 Client 内写向相同本地目标的缓存事务，避免并发
	// 下载把不同响应体或 validator sidecar 交叉提交。
	resourceDownloads resourceDownloadLocks
	resourceCachePath string
	httpClient        *http.Client
	authFilePath      string
	configFilePath    string
	oauthBaseURL      string
	appAPIBaseURL     string
	authState         *authTransactionState
	// defaults 非 nil 表示每个公开操作都必须取得一个新的本地快照。
	defaults *defaultOptions
	// cursorSource 只存在于 OpenDefault 的 operation-scoped client；它从不含凭据。
	cursorSource string
	// authenticatedUserID 仅由 OpenDefault 的 OAuth 快照写入；显式 access token
	// 不声称可从 token 本身推断用户身份。
	authenticatedUserID    int64
	premiumStatusMu        sync.Mutex
	cachedPremiumStatus    *bool
	premiumStatusCheckedAt time.Time
	premiumStatusCacheTTL  time.Duration
	premiumStatusAuthPath  string
	logger                 *slog.Logger
}

// CurrentUserID 返回 OpenDefault 当前认证快照对应的 Pixiv UID。
//
// 它为 CLI 等需要将省略的 USER_ID 解释为“我自己”的调用方提供身份边界；不会把
// 本地默认账号错误地当作显式 refresh token 或环境变量 token 的身份。显式 NewClient
// 的 access token 不携带可验证 UID，因此返回 unsupported。
func (c *Client) CurrentUserID(ctx context.Context) (userID int64, err error) {
	started := time.Now()
	if c.defaults != nil {
		scoped, snapshotErr := c.operationClient(ctx, OperationCurrentUserID)
		if snapshotErr != nil {
			return 0, snapshotErr
		}
		userID, err = scoped.currentUserID()
		// operationClient 已记录快照失败；成功后的身份解析只有这里一条事件。
		scoped.operationLog(OperationCurrentUserID, started, err, 0, userID)
		return userID, err
	}
	defer func() { c.operationLog(OperationCurrentUserID, started, err, 0, userID) }()
	if scoped, err := c.operationClient(ctx, OperationCurrentUserID); err != nil {
		return 0, err
	} else if scoped != c {
		return scoped.currentUserID()
	}
	return c.currentUserID()
}

// Snapshot 取得一次明确的本地配置与认证快照，供一个高层操作内的多个
// cursor 请求复用。OpenDefault 的普通公开方法仍各自刷新快照；调用方只有显式
// 选择本方法时才把同一快照固定在返回 Client 上。
func (c *Client) Snapshot(ctx context.Context) (*Client, error) {
	if c.defaults == nil {
		return c, nil
	}
	return c.operationClient(ctx, OperationSnapshot)
}

func (c *Client) currentUserID() (int64, error) {
	if !c.authenticated {
		return 0, localRouteError(CodeUnauthorized, OperationCurrentUserID, 0, 0, errors.New("access token is required"))
	}
	if c.authenticatedUserID <= 0 {
		return 0, localRouteError(CodeUnsupported, OperationCurrentUserID, 0, 0, errors.New("authenticated user identity is unavailable"))
	}
	return c.authenticatedUserID, nil
}

// NewClient 构造显式 access-token 或匿名客户端；它不会执行网络请求或隐式认证。
func NewClient(options NewClientOptions) (*Client, error) {
	return newClient(clientOptions{
		Logger: options.Logger, HTTPClient: options.HTTPClient, AppAPIBaseURL: options.AppAPIBaseURL,
		WebAPIBaseURL: options.WebAPIBaseURL, AccessToken: options.AccessToken,
		WebFallbackEnabled: options.WebFallbackEnabled, ResourcePolicy: options.ResourcePolicy, ResourceCachePath: options.ResourceCachePath,
		DisableRetryAfterRetry: options.DisableRetryAfterRetry,
	})
}

func newClient(options clientOptions) (*Client, error) {
	resourcePolicy, err := compileResourcePolicy(options.ResourcePolicy)
	if err != nil {
		return nil, err
	}
	accessToken := strings.TrimSpace(options.AccessToken)
	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	appOptions := []appapi.Option{
		appapi.WithBaseURL(options.AppAPIBaseURL),
		appapi.WithAccessToken(accessToken),
		appapi.WithHTTPClient(httpClient),
		appapi.WithLogger(options.Logger),
	}
	if options.DisableRetryAfterRetry {
		appOptions = append(appOptions, appapi.WithDisableRetryAfterRetry())
	}
	webOptions := []webapi.Option{
		webapi.WithWebBase(options.WebAPIBaseURL),
		webapi.WithHTTPClient(httpClient),
	}
	resourceHTTPClient := resourceHTTPClientForExplicitProxy(httpClient)
	return &Client{
		app:                appapi.New(appOptions...),
		web:                webapi.New(webOptions...),
		authenticated:      accessToken != "",
		webFallbackEnabled: options.WebFallbackEnabled,
		resourcePolicy:     resourcePolicy,
		resource:           internalresource.NewApp(resourceHTTPClient),
		resourceCachePath:  strings.TrimSpace(options.ResourceCachePath),
		httpClient:         httpClient,
		authFilePath:       strings.TrimSpace(options.AuthFilePath),
		configFilePath:     strings.TrimSpace(options.ConfigFilePath),
		oauthBaseURL:       strings.TrimSpace(options.OAuthBaseURL),
		appAPIBaseURL:      strings.TrimSpace(options.AppAPIBaseURL),
		authState:          &authTransactionState{},
		logger:             logging.OrDiscard(options.Logger),
	}, nil
}

// resourceHTTPClientForExplicitProxy 为显式代理的资源流单独复制 transport。
// 一些本地 HTTP(S) 代理会在 HTTP/2 资源流中断开连接；控制面仍保持调用方原有
// 协议协商，只有媒体传输固定为 HTTP/1.1。
func resourceHTTPClientForExplicitProxy(httpClient *http.Client) *http.Client {
	transport, ok := httpClient.Transport.(*http.Transport)
	if !ok || transport.Proxy == nil {
		return httpClient
	}
	resourceClient := *httpClient
	resourceTransport := transport.Clone()
	resourceTransport.ForceAttemptHTTP2 = false
	// Transport.Clone 会完成源 transport 的协议初始化，可能已把 h2 写入
	// TLSClientConfig。显式清除该协商并安装空 TLSNextProto，才能确保此副本
	// 不会因后续 RoundTrip 再次自动启用 HTTP/2。
	resourceTransport.TLSNextProto = make(map[string]func(string, *tls.Conn) http.RoundTripper)
	if resourceTransport.TLSClientConfig == nil {
		resourceTransport.TLSClientConfig = &tls.Config{}
	}
	resourceTransport.TLSClientConfig.NextProtos = []string{"http/1.1"}
	resourceClient.Transport = resourceTransport
	return &resourceClient
}

// OpenDefault 构造使用默认本地 auth.json、config.toml 与环境变量的客户端。
func OpenDefault() (*Client, error) { return OpenDefaultWith(OpenDefaultOptions{}) }

// OpenDefaultWith 构造使用显式本地路径、身份选择或 transport 覆写的默认客户端。
// 它不缓存这些状态：每次公开操作开始时都会取得一次新快照。
func OpenDefaultWith(input OpenDefaultOptions) (*Client, error) {
	options := clientOptions{
		Logger: input.Logger, HTTPClient: input.HTTPClient, AppAPIBaseURL: input.AppAPIBaseURL,
		WebAPIBaseURL: input.WebAPIBaseURL, OAuthBaseURL: input.OAuthBaseURL,
		AuthFilePath: input.AuthFilePath, ConfigFilePath: input.ConfigFilePath,
		ResourcePolicy: input.ResourcePolicy, ResourceCachePath: input.ResourceCachePath, RefreshToken: input.RefreshToken, UserID: input.UserID,
		IgnoreEnvironmentRefreshToken: input.IgnoreEnvironmentRefreshToken, DisableRetryAfterRetry: input.DisableRetryAfterRetry,
	}
	if _, err := credentials.ValidateRefreshTokenInput(options.RefreshToken); err != nil {
		return nil, newError(CodeInvalidArgument, "", "", false, 0, 0, err)
	}
	options = cloneClientOptions(options)
	baseOptions := options
	baseOptions.AccessToken = ""
	base, err := newClient(baseOptions)
	if err != nil {
		return nil, err
	}
	base.defaults = &defaultOptions{options: options, authState: base.authState}
	return base, nil
}

type authTransactionState struct{ mu sync.Mutex }

func cloneClientOptions(options clientOptions) clientOptions {
	policy := ResourcePolicy{Mirrors: make([]ResourceMirrorPolicy, len(options.ResourcePolicy.Mirrors))}
	for index, mirror := range options.ResourcePolicy.Mirrors {
		policy.Mirrors[index] = ResourceMirrorPolicy{Host: mirror.Host, PathPrefixes: append([]string(nil), mirror.PathPrefixes...)}
	}
	options.ResourcePolicy = policy
	return options
}

// newHTTPClientForSnapshot 保留显式 transport；否则把当前配置中的代理绑定到本次操作。
func newHTTPClientForSnapshot(options clientOptions, proxy string) (*http.Client, error) {
	if options.HTTPClient != nil {
		return options.HTTPClient, nil
	}
	return internalpixiv.HTTPClient(proxy)
}

// IllustDetail 在认证态使用 App API 返回的详情和页面元数据；App 失败时不会请求 Web。
func (c *Client) IllustDetail(ctx context.Context, id int64) (result *IllustDetail, err error) {
	started := time.Now()
	defer func() { c.delegatedOperationLog(OperationIllustDetail, started, err, id, 0) }()
	if scoped, err := c.operationClient(ctx, OperationIllustDetail); err != nil {
		return nil, err
	} else if scoped != c {
		return scoped.IllustDetail(ctx, id)
	}
	if id <= 0 {
		return nil, newError(
			CodeInvalidArgument,
			OperationIllustDetail,
			"",
			false,
			0,
			0,
			errors.New("illust id must be positive"),
		)
	}
	route, err := c.selectRoute(OperationIllustDetail, id, 0)
	if err != nil {
		return nil, err
	}
	if route == routeWeb {
		detail, err := c.web.IllustDetail(ctx, id)
		if err != nil {
			return nil, mapWebError(err, OperationIllustDetail, id)
		}
		result := mapIllustDetail(*detail)
		return &result, nil
	}
	if route != routeApp {
		return nil, unexpectedRoute(OperationIllustDetail, id, 0)
	}
	detail, err := c.app.IllustDetail(ctx, id)
	if err != nil {
		return nil, mapAppError(err, OperationIllustDetail, id)
	}
	pages, err := appDetailMetaPages(detail.Illust)
	if err != nil {
		return nil, mapAppError(err, OperationIllustDetail, id)
	}
	out := mapIllustDetail(*detail)
	out.Illust.MetaPages = pages
	return &out, nil
}

func mapWebError(err error, operation Operation, illustID int64) error {
	var pagesError *webapi.IllustPagesError
	if errors.As(err, &pagesError) {
		return mapWebError(errors.Unwrap(pagesError), OperationIllustPages, illustID)
	}
	return mapAdapterFailure(err, operation, BackendWebAPI, illustID, 0)
}

func mapAppError(err error, operation Operation, illustID int64) error {
	return mapAdapterFailure(err, operation, BackendAppAPI, illustID, 0)
}

// mapAdapterFailure 是公开 SDK 接收 adapter 失败的唯一映射点。adapter 只交付
// protocol.Failure；未知错误也按 transport 处理，因此原始 URL/body/凭据不会进入
// *Error 的 Error 或 Unwrap。
func mapAdapterFailure(err error, operation Operation, backend Backend, illustID, userID int64) error {
	code, retryable, status := CodeUpstreamUnavailable, true, 0
	cause := error(errors.New("upstream transport failed"))
	transportKind := TransportKind("")
	retryAfter := time.Duration(0)
	hasRetryAfter := false
	var failure protocol.Failure
	if errors.As(err, &failure) {
		switch failure.Kind {
		case protocol.FailureHTTPStatus:
			code, retryable = codeForHTTPStatus(failure.StatusCode, operation)
			status = failure.StatusCode
			if failure.StatusCode == http.StatusTooManyRequests && failure.HasRetryAfter {
				retryAfter, hasRetryAfter = failure.RetryAfter, true
			}
			cause = fmt.Errorf("upstream returned HTTP status %d", failure.StatusCode)
		case protocol.FailureMalformed:
			code, retryable = CodeMalformedUpstreamResponse, false
			cause = errors.New("upstream response was malformed")
		case protocol.FailureRejected:
			code, retryable = CodeUpstreamError, true
			cause = errors.New("upstream rejected the request")
			if backend == BackendWebAPI && operation == OperationIllustDetail {
				code, retryable = CodeArtworkUnavailable, false
				cause = errors.New("artwork is unavailable from web api")
			}
		case protocol.FailureForbidden:
			code, retryable = CodeForbidden, false
			cause = errors.New("request was forbidden by policy")
		case protocol.FailureTransport:
			transportKind = TransportKind(failure.TransportKind)
			if errors.Is(failure, context.Canceled) {
				retryable, cause = false, context.Canceled
			} else if errors.Is(failure, context.DeadlineExceeded) {
				retryable, cause = false, context.DeadlineExceeded
			}
		}
	} else if errors.Is(err, protocol.ErrMalformedResponse) {
		code, retryable = CodeMalformedUpstreamResponse, false
		cause = errors.New("upstream response was malformed")
	} else if errors.Is(err, context.Canceled) {
		retryable, cause = false, context.Canceled
	} else if errors.Is(err, context.DeadlineExceeded) {
		retryable, cause = false, context.DeadlineExceeded
	}
	if userID > 0 {
		mapped := newUserError(code, operation, backend, retryable, status, userID, cause)
		mapped.TransportKind = transportKind
		mapped.RetryAfter, mapped.HasRetryAfter = retryAfter, hasRetryAfter
		return mapped
	}
	mapped := newError(code, operation, backend, retryable, status, illustID, cause)
	mapped.TransportKind = transportKind
	mapped.RetryAfter, mapped.HasRetryAfter = retryAfter, hasRetryAfter
	return mapped
}

func codeForHTTPStatus(status int, operation Operation) (ErrorCode, bool) {
	switch status {
	case http.StatusBadRequest:
		return CodeInvalidArgument, false
	case http.StatusUnauthorized:
		return CodeUnauthorized, false
	case http.StatusForbidden:
		return CodeForbidden, false
	case http.StatusNotFound:
		if operation == OperationIllustDetail {
			return CodeArtworkUnavailable, false
		}
		return CodeUpstreamError, true
	case http.StatusTooManyRequests:
		return CodeRateLimited, true
	case http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return CodeUpstreamUnavailable, true
	default:
		return CodeUpstreamError, true
	}
}

func mapIllustDetail(detail model.IllustDetail) IllustDetail {
	return IllustDetail{Illust: mapIllust(detail.Illust)}
}

func mapIllust(illust model.Illust) Illust {
	tags := make([]Tag, len(illust.Tags))
	for index, tag := range illust.Tags {
		tags[index] = Tag{Name: tag.Name, TranslatedName: tag.TranslatedName}
	}
	return Illust{
		URL:            artworkURL(illust.ID),
		ID:             illust.ID,
		Title:          illust.Title,
		Caption:        illust.Caption,
		Type:           illust.Type,
		PageCount:      illust.PageCount,
		TotalBookmarks: illust.TotalBookmarks,
		TotalView:      illust.TotalView,
		XRestrict:      illust.XRestrict,
		User: User{
			ID:         illust.User.ID,
			Name:       illust.User.Name,
			Account:    illust.User.Account,
			Comment:    illust.User.Comment,
			IsFollowed: illust.User.IsFollowed,
		},
		Tags:           tags,
		ImageURLs:      mapImageURLs(illust.ImageURLs),
		MetaSinglePage: SinglePage{OriginalImageURL: illust.MetaSinglePage.OriginalImageURL},
		MetaPages:      mapMetaPages(illust.MetaPages),
		AIType:         illust.AIType,
		CreateDate:     illust.CreateDate,
		Width:          illust.Width,
		Height:         illust.Height,
		Tools:          append([]string{}, illust.Tools...),
	}
}

func mapNovel(novel model.Novel) Novel {
	tags := make([]Tag, len(novel.Tags))
	for index, tag := range novel.Tags {
		tags[index] = Tag{Name: tag.Name, TranslatedName: tag.TranslatedName}
	}
	return Novel{
		URL: novelURL(novel.ID), ID: novel.ID, Title: novel.Title, Caption: novel.Caption,
		XRestrict: novel.XRestrict, TextLength: novel.TextLength, IsOriginal: novel.IsOriginal, User: mapUser(novel.User), Tags: tags,
		ImageURLs: mapImageURLs(novel.ImageURLs), CreateDate: novel.CreateDate, TotalBookmarks: novel.TotalBookmarks, TotalView: novel.TotalView,
	}
}

func novelURL(id int64) string {
	if id <= 0 {
		return ""
	}
	return "https://www.pixiv.net/novel/show.php?id=" + fmt.Sprint(id)
}

func mapMetaPages(pages []model.MetaPage) []MetaPage {
	result := make([]MetaPage, len(pages))
	for index, page := range pages {
		result[index] = MetaPage{
			PageIndex: page.PageIndex,
			Width:     page.Width,
			Height:    page.Height,
			Extension: page.Extension,
			ImageURLs: mapImageURLs(page.ImageURLs),
		}
	}
	return result
}

func mapImageURLs(urls model.ImageURLs) ImageURLs {
	return ImageURLs{
		SquareMedium: urls.SquareMedium,
		Medium:       urls.Medium,
		Large:        urls.Large,
		Original:     urls.Original,
	}
}

func artworkURL(id int64) string {
	if id <= 0 {
		return ""
	}
	return "https://www.pixiv.net/artworks/" + fmt.Sprint(id)
}
