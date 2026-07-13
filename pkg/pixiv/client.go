// Package pixiv 提供可嵌入 Go 程序的 Pixiv 客户端与稳定模型。
package pixiv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	internalpixiv "github.com/FlanChanXwO/pixiv-cli/internal/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/appapi"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/model"
	internalresource "github.com/FlanChanXwO/pixiv-cli/internal/pixiv/resource"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/webapi"
)

// Options 配置 Client 当前所需的传输端点与 App API 身份。
type Options struct {
	// Logger 接收调用方显式注入的诊断 logger；为空时 SDK 严格静默，绝不使用 slog.Default。
	Logger *slog.Logger
	// HTTPClient 同时承载 App 与 Web 请求；为空时使用各内部客户端默认值。
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
}

// Client 组合 App API 主数据与显式 Web 补全能力。
type Client struct {
	app                *appapi.Client
	web                *webapi.Client
	authenticated      bool
	webFallbackEnabled bool
	resourcePolicy     resourcePolicy
	resource           *internalresource.Client
	httpClient         *http.Client
	authFilePath       string
	configFilePath     string
	oauthBaseURL       string
	appAPIBaseURL      string
	authState          *authTransactionState
	// defaults 非 nil 表示每个公开操作都必须取得一个新的本地快照。
	defaults *defaultOptions
	// cursorSource 只存在于 OpenDefault 的 operation-scoped client；它从不含凭据。
	cursorSource string
	// authenticatedUserID 仅由 OpenDefault 的 OAuth 快照写入；显式 access token
	// 不声称可从 token 本身推断用户身份。
	authenticatedUserID int64
	logger              *slog.Logger
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

// NewClient 构造具体客户端；它不会执行网络请求或隐式认证。
func NewClient(options Options) (*Client, error) {
	resourcePolicy, err := compileResourcePolicy(options.ResourcePolicy)
	if err != nil {
		return nil, err
	}
	accessToken := strings.TrimSpace(options.AccessToken)
	appOptions := []appapi.Option{
		appapi.WithBaseURL(options.AppAPIBaseURL),
		appapi.WithAccessToken(accessToken),
	}
	webOptions := []webapi.Option{webapi.WithWebBase(options.WebAPIBaseURL)}
	if options.HTTPClient != nil {
		appOptions = append(appOptions, appapi.WithHTTPClient(options.HTTPClient))
		webOptions = append(webOptions, webapi.WithHTTPClient(options.HTTPClient))
	}
	return &Client{
		app:                appapi.New(appOptions...),
		web:                webapi.New(webOptions...),
		authenticated:      accessToken != "",
		webFallbackEnabled: options.WebFallbackEnabled,
		resourcePolicy:     resourcePolicy,
		resource:           internalresource.NewApp(options.HTTPClient),
		httpClient:         options.HTTPClient,
		authFilePath:       strings.TrimSpace(options.AuthFilePath),
		configFilePath:     strings.TrimSpace(options.ConfigFilePath),
		oauthBaseURL:       strings.TrimSpace(options.OAuthBaseURL),
		appAPIBaseURL:      strings.TrimSpace(options.AppAPIBaseURL),
		authState:          &authTransactionState{},
		logger:             loggerOrDiscard(options.Logger),
	}, nil
}

func loggerOrDiscard(logger *slog.Logger) *slog.Logger {
	if logger != nil {
		return logger
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// OpenDefault 构造使用本地 auth.json、config.toml 与环境变量的客户端。
// 它不缓存这些状态：每次公开操作开始时都会取得一次新快照。
func OpenDefault(options Options) (*Client, error) {
	if strings.TrimSpace(options.AccessToken) != "" {
		return nil, newError(CodeInvalidArgument, "", "", false, 0, 0, errors.New("AccessToken is only supported by NewClient"))
	}
	options = cloneOptions(options)
	baseOptions := options
	baseOptions.AccessToken = ""
	base, err := NewClient(baseOptions)
	if err != nil {
		return nil, err
	}
	base.defaults = &defaultOptions{options: options, authState: base.authState}
	return base, nil
}

type authTransactionState struct{ mu sync.Mutex }

func cloneOptions(options Options) Options {
	policy := ResourcePolicy{Mirrors: make([]ResourceMirrorPolicy, len(options.ResourcePolicy.Mirrors))}
	for index, mirror := range options.ResourcePolicy.Mirrors {
		policy.Mirrors[index] = ResourceMirrorPolicy{Host: mirror.Host, PathPrefixes: append([]string(nil), mirror.PathPrefixes...)}
	}
	options.ResourcePolicy = policy
	return options
}

// newHTTPClientForSnapshot 保留显式 transport；否则把当前配置中的代理绑定到本次操作。
func newHTTPClientForSnapshot(options Options, proxy string) (*http.Client, error) {
	if options.HTTPClient != nil {
		return options.HTTPClient, nil
	}
	return internalpixiv.HTTPClient(proxy)
}

// IllustDetail 先读取 App API 详情，再用 Web pages 显式补全页面元数据。
// App 失败时不会请求 Web；Web 补全失败会直接返回错误。
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
	if route != routeAppThenWeb {
		return nil, unexpectedRoute(OperationIllustDetail, id, 0)
	}
	detail, err := c.app.IllustDetail(ctx, id)
	if err != nil {
		return nil, mapAppError(err, OperationIllustDetail, id)
	}
	pages, err := c.web.IllustPages(ctx, id)
	if err != nil {
		return nil, mapWebError(err, OperationIllustPages, id)
	}

	out := mapIllustDetail(*detail)
	out.Illust.MetaPages = mapMetaPages(pages)
	return &out, nil
}

func mapWebError(err error, operation Operation, illustID int64) error {
	var pagesError *webapi.IllustPagesError
	if errors.As(err, &pagesError) {
		return mapWebError(errors.Unwrap(pagesError), OperationIllustPages, illustID)
	}
	var upstream webapi.APIError
	if errors.As(err, &upstream) {
		code, retryable := codeForHTTPStatus(upstream.StatusCode, operation)
		return newError(
			code,
			operation,
			BackendWebAPI,
			retryable,
			upstream.StatusCode,
			illustID,
			fmt.Errorf("upstream returned HTTP status %d", upstream.StatusCode),
		)
	}
	if errors.Is(err, webapi.ErrMalformedResponse) {
		return malformedError(operation, BackendWebAPI, illustID)
	}
	var envelope webapi.EnvelopeError
	if errors.As(err, &envelope) {
		if operation == OperationIllustDetail {
			return newError(CodeArtworkUnavailable, operation, BackendWebAPI, false, 0, illustID, errors.New("artwork is unavailable from web api"))
		}
		return newError(CodeUpstreamError, operation, BackendWebAPI, true, 0, illustID, errors.New("web api rejected the request"))
	}
	return transportError(err, operation, BackendWebAPI, illustID)
}

func mapAppError(err error, operation Operation, illustID int64) error {
	var upstream appapi.APIError
	if errors.As(err, &upstream) {
		code, retryable := codeForHTTPStatus(upstream.StatusCode, operation)
		return newError(
			code,
			operation,
			BackendAppAPI,
			retryable,
			upstream.StatusCode,
			illustID,
			fmt.Errorf("upstream returned HTTP status %d", upstream.StatusCode),
		)
	}
	if errors.Is(err, appapi.ErrMalformedResponse) {
		return malformedError(operation, BackendAppAPI, illustID)
	}
	return transportError(err, operation, BackendAppAPI, illustID)
}

func malformedError(operation Operation, backend Backend, illustID int64) error {
	return newError(
		CodeMalformedUpstreamResponse,
		operation,
		backend,
		false,
		0,
		illustID,
		errors.New("upstream response was malformed"),
	)
}

func transportError(err error, operation Operation, backend Backend, illustID int64) error {
	if errors.Is(err, context.Canceled) {
		return newError(CodeUpstreamUnavailable, operation, backend, false, 0, illustID, context.Canceled)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return newError(CodeUpstreamUnavailable, operation, backend, false, 0, illustID, context.DeadlineExceeded)
	}
	return newError(CodeUpstreamUnavailable, operation, backend, true, 0, illustID, errors.New("upstream transport failed"))
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
		ID:             illust.ID,
		Title:          illust.Title,
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
	}
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
