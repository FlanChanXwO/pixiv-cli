// Package pixiv 提供可嵌入 Go 程序的 Pixiv 客户端与稳定模型。
package pixiv

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/appapi"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/model"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/webapi"
)

// Options 配置 Client 当前所需的传输端点与 App API 身份。
type Options struct {
	// HTTPClient 同时承载 App 与 Web 请求；为空时使用各内部客户端默认值。
	HTTPClient *http.Client
	// AppAPIBaseURL 覆盖 App API 地址，主要用于代理与测试。
	AppAPIBaseURL string
	// WebAPIBaseURL 覆盖 Pixiv Web 地址，主要用于页面补全与测试。
	WebAPIBaseURL string
	// AccessToken 是调用 App API 使用的 bearer token。
	AccessToken string
	// WebFallbackEnabled 允许无 access token 时显式使用匿名 Web API。
	WebFallbackEnabled bool
}

// Client 组合 App API 主数据与显式 Web 补全能力。
type Client struct {
	app                *appapi.Client
	web                *webapi.Client
	authenticated      bool
	webFallbackEnabled bool
}

// NewClient 构造具体客户端；它不会执行网络请求或隐式认证。
func NewClient(options Options) (*Client, error) {
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
	}, nil
}

// IllustDetail 先读取 App API 详情，再用 Web pages 显式补全页面元数据。
// App 失败时不会请求 Web；Web 补全失败会直接返回错误。
func (c *Client) IllustDetail(ctx context.Context, id int64) (*IllustDetail, error) {
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

	result := mapIllustDetail(*detail)
	result.Illust.MetaPages = mapMetaPages(pages)
	return &result, nil
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
