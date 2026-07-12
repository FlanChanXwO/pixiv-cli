// Package pixiv 提供可嵌入 Go 程序的 Pixiv 客户端与稳定模型。
package pixiv

import (
	"context"
	"fmt"
	"net/http"

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
}

// Client 组合 App API 主数据与显式 Web 补全能力。
type Client struct {
	app *appapi.Client
	web *webapi.Client
}

type operation uint8

const opIllustDetail operation = iota

// webEnrichmentEnabled 是 SDK 允许 Web 补全的唯一 operation policy。
func webEnrichmentEnabled(op operation) bool {
	return op == opIllustDetail
}

// NewClient 构造具体客户端；它不会执行网络请求或隐式认证。
func NewClient(options Options) (*Client, error) {
	appOptions := []appapi.Option{
		appapi.WithBaseURL(options.AppAPIBaseURL),
		appapi.WithAccessToken(options.AccessToken),
	}
	webOptions := []webapi.Option{webapi.WithWebBase(options.WebAPIBaseURL)}
	if options.HTTPClient != nil {
		appOptions = append(appOptions, appapi.WithHTTPClient(options.HTTPClient))
		webOptions = append(webOptions, webapi.WithHTTPClient(options.HTTPClient))
	}
	return &Client{
		app: appapi.New(appOptions...),
		web: webapi.New(webOptions...),
	}, nil
}

// IllustDetail 先读取 App API 详情，再用 Web pages 显式补全页面元数据。
// App 失败时不会请求 Web；Web 补全失败会直接返回错误。
func (c *Client) IllustDetail(ctx context.Context, id int64) (*IllustDetail, error) {
	detail, err := c.app.IllustDetail(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("app illust detail: %w", err)
	}
	if !webEnrichmentEnabled(opIllustDetail) {
		return nil, fmt.Errorf("web enrichment is not enabled for illust detail")
	}
	pages, err := c.web.IllustPages(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("web illust pages: %w", err)
	}

	result := mapIllustDetail(*detail)
	result.Illust.MetaPages = mapMetaPages(pages)
	return &result, nil
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
