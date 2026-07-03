package pixiv

import (
	"net/http"

	"github.com/FlanChanXwO/pixiv-mcp-server/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-mcp-server/internal/pixiv/model"
)

const (
	DefaultAPIBase           = api.DefaultAPIBase
	DefaultOAuthBase         = api.DefaultOAuthBase
	DefaultOAuthClientID     = api.DefaultOAuthClientID
	DefaultOAuthClientSecret = api.DefaultOAuthClientSecret
	DefaultOAuthRedirectURI  = api.DefaultOAuthRedirectURI
	DefaultUserAgent         = api.DefaultUserAgent
	DefaultAppOS             = api.DefaultAppOS
	DefaultAppOSVersion      = api.DefaultAppOSVersion
	DefaultAppVersion        = api.DefaultAppVersion
)

type (
	Client = api.Client
	Option = api.Option

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

func New(refreshToken string, opts ...Option) *Client {
	return api.New(refreshToken, opts...)
}

func WithHTTPClient(httpClient *http.Client) Option {
	return api.WithHTTPClient(httpClient)
}

func WithBaseURLs(apiBase, oauthBase string) Option {
	return api.WithBaseURLs(apiBase, oauthBase)
}
