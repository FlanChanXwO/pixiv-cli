package pixiv

import (
	"net/http"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/appapi"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/model"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/oauth"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/resource"
)

const (
	DefaultAPIBase           = appapi.DefaultAPIBase
	DefaultOAuthBase         = oauth.DefaultBase
	DefaultOAuthClientID     = oauth.DefaultClientID
	DefaultOAuthClientSecret = oauth.DefaultClientSecret
	DefaultOAuthRedirectURI  = oauth.DefaultRedirectURI
	DefaultUserAgent         = appapi.DefaultUserAgent
	DefaultAppOS             = appapi.DefaultAppOS
	DefaultAppOSVersion      = appapi.DefaultAppOSVersion
	DefaultAppVersion        = appapi.DefaultAppVersion
)

type (
	Client               = Source
	Illust               = model.Illust
	IllustList           = model.IllustList
	IllustDetail         = model.IllustDetail
	User                 = model.User
	Tag                  = model.Tag
	ImageURLs            = model.ImageURLs
	SinglePage           = model.SinglePage
	MetaPage             = model.MetaPage
	UserPreviewList      = model.UserPreviewList
	UserPreview          = model.UserPreview
	TrendTags            = model.TrendTags
	TrendTag             = model.TrendTag
	UgoiraMetadataResult = model.UgoiraMetadataResult
	UgoiraMetadata       = model.UgoiraMetadata
	UgoiraFrame          = model.UgoiraFrame
	SearchTarget         = model.SearchTarget
	SortMode             = model.SortMode
	RankingMode          = model.RankingMode
	Restrict             = model.Restrict
	IllustType           = model.IllustType
)

const (
	SearchTargetPartialMatchForTags = model.SearchTargetPartialMatchForTags
	SearchTargetExactMatchForTags   = model.SearchTargetExactMatchForTags
	SearchTargetTitleAndCaption     = model.SearchTargetTitleAndCaption
	SortModeDateDesc                = model.SortModeDateDesc
	SortModeDateAsc                 = model.SortModeDateAsc
	RankingModeDay                  = model.RankingModeDay
	RankingModeWeek                 = model.RankingModeWeek
	RankingModeMonth                = model.RankingModeMonth
	RestrictPublic                  = model.RestrictPublic
	RestrictPrivate                 = model.RestrictPrivate
	IllustTypeIllust                = model.IllustTypeIllust
	IllustTypeManga                 = model.IllustTypeManga
	IllustTypeUgoira                = model.IllustTypeUgoira
)

type legacyOptions struct {
	httpClient         *http.Client
	apiBase, oauthBase string
}
type Option func(*legacyOptions)

func WithHTTPClient(client *http.Client) Option {
	return func(o *legacyOptions) { o.httpClient = client }
}
func WithBaseURLs(apiBase, oauthBase string) Option {
	return func(o *legacyOptions) {
		o.apiBase, o.oauthBase = strings.TrimRight(apiBase, "/"), strings.TrimRight(oauthBase, "/")
	}
}

// New 保留 internal CLI 测试与组装所需入口；协议实现已分别落在 appapi/oauth/resource。
func New(refreshToken string, opts ...Option) *Client {
	var cfg legacyOptions
	for _, opt := range opts {
		opt(&cfg)
	}
	auth := oauth.New(refreshToken, oauth.WithHTTPClient(cfg.httpClient), oauth.WithBaseURL(cfg.oauthBase))
	app := appapi.New(appapi.WithHTTPClient(cfg.httpClient), appapi.WithBaseURL(cfg.apiBase), appapi.WithSession(auth))
	return NewSourceFromClients(app, nil, auth, resource.NewApp(cfg.httpClient), nil, SourcePolicy{RefreshToken: refreshToken})
}
