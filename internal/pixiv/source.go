package pixiv

import (
	"context"
	"io"
	"strings"
)

type AppAPI interface {
	SearchIllust(context.Context, string, string, string, string, int) (*IllustList, error)
	IllustDetail(context.Context, int64) (*IllustDetail, error)
	IllustRelated(context.Context, int64, int) (*IllustList, error)
	IllustRanking(context.Context, string, string, int) (*IllustList, error)
	SearchUser(context.Context, string, int) (*UserPreviewList, error)
	UserDetail(context.Context, int64) (*User, error)
	IllustRecommended(context.Context, int) (*IllustList, error)
	TrendingTagsIllust(context.Context) (*TrendTags, error)
	IllustFollow(context.Context, string, int) (*IllustList, error)
	UserBookmarks(context.Context, int64, string, string, int64) (*IllustList, error)
	UserFollowing(context.Context, int64, string, int) (*UserPreviewList, error)
	UgoiraMetadata(context.Context, int64) (*UgoiraMetadataResult, error)
}

type WebAPI interface {
	SearchIllust(context.Context, string, string, string, string, int) (*IllustList, error)
	IllustDetail(context.Context, int64) (*IllustDetail, error)
	IllustRanking(context.Context, string, string, int) (*IllustList, error)
	SearchUser(context.Context, string, int) (*UserPreviewList, error)
	UgoiraMetadata(context.Context, int64) (*UgoiraMetadataResult, error)
}

type AuthAPI interface {
	Refresh(context.Context) error
	SetRefreshToken(string)
	RefreshTokenValue() string
	UserID() int64
	UserName() string
	IsAuthenticated() bool
}

type ResourceAPI interface {
	Download(context.Context, string, io.Writer) error
}

type SourcePolicy struct {
	RefreshToken       string
	WebFallbackEnabled bool
}

type operation uint8

const (
	opSearchIllust operation = iota
	opIllustDetail
	opIllustRanking
	opSearchUser
	opUgoiraMetadata
	opDownload
)

// isWebOperation 是匿名 Web 能力的唯一白名单；未列出的内容操作始终走 App。
func isWebOperation(op operation) bool {
	switch op {
	case opSearchIllust, opIllustDetail, opIllustRanking, opSearchUser, opUgoiraMetadata, opDownload:
		return true
	default:
		return false
	}
}

type Source struct {
	app         AppAPI
	web         WebAPI
	auth        AuthAPI
	appResource ResourceAPI
	webResource ResourceAPI
	webEnabled  bool
}

func NewSourceFromClients(app AppAPI, web WebAPI, auth AuthAPI, appResource, webResource ResourceAPI, cfg SourcePolicy) *Source {
	return &Source{app: app, web: web, auth: auth, appResource: appResource, webResource: webResource,
		webEnabled: strings.TrimSpace(cfg.RefreshToken) == "" && cfg.WebFallbackEnabled}
}

func (s *Source) Refresh(ctx context.Context) error { return s.auth.Refresh(ctx) }
func (s *Source) SetRefreshToken(token string)      { s.auth.SetRefreshToken(token) }
func (s *Source) RefreshTokenValue() string         { return s.auth.RefreshTokenValue() }
func (s *Source) UserID() int64                     { return s.auth.UserID() }
func (s *Source) UserName() string                  { return s.auth.UserName() }
func (s *Source) IsAuthenticated() bool             { return s.auth.IsAuthenticated() }

func (s *Source) useWeb(op operation) bool {
	if !s.webEnabled || s.web == nil || strings.TrimSpace(s.auth.RefreshTokenValue()) != "" {
		return false
	}
	return isWebOperation(op)
}

func (s *Source) SearchIllust(ctx context.Context, word, target, sort, duration string, offset int) (*IllustList, error) {
	if s.useWeb(opSearchIllust) {
		return s.web.SearchIllust(ctx, word, target, sort, duration, offset)
	}
	return s.app.SearchIllust(ctx, word, target, sort, duration, offset)
}
func (s *Source) IllustDetail(ctx context.Context, id int64) (*IllustDetail, error) {
	if s.useWeb(opIllustDetail) {
		return s.web.IllustDetail(ctx, id)
	}
	return s.app.IllustDetail(ctx, id)
}
func (s *Source) IllustRelated(ctx context.Context, id int64, offset int) (*IllustList, error) {
	return s.app.IllustRelated(ctx, id, offset)
}
func (s *Source) IllustRanking(ctx context.Context, mode, date string, offset int) (*IllustList, error) {
	if s.useWeb(opIllustRanking) {
		return s.web.IllustRanking(ctx, mode, date, offset)
	}
	return s.app.IllustRanking(ctx, mode, date, offset)
}
func (s *Source) SearchUser(ctx context.Context, word string, offset int) (*UserPreviewList, error) {
	if s.useWeb(opSearchUser) {
		return s.web.SearchUser(ctx, word, offset)
	}
	return s.app.SearchUser(ctx, word, offset)
}
func (s *Source) UserDetail(ctx context.Context, id int64) (*User, error) {
	return s.app.UserDetail(ctx, id)
}
func (s *Source) IllustRecommended(ctx context.Context, offset int) (*IllustList, error) {
	return s.app.IllustRecommended(ctx, offset)
}
func (s *Source) TrendingTagsIllust(ctx context.Context) (*TrendTags, error) {
	return s.app.TrendingTagsIllust(ctx)
}
func (s *Source) IllustFollow(ctx context.Context, restrict string, offset int) (*IllustList, error) {
	return s.app.IllustFollow(ctx, restrict, offset)
}
func (s *Source) UserBookmarks(ctx context.Context, id int64, restrict, tag string, max int64) (*IllustList, error) {
	return s.app.UserBookmarks(ctx, id, restrict, tag, max)
}
func (s *Source) UserFollowing(ctx context.Context, id int64, restrict string, offset int) (*UserPreviewList, error) {
	return s.app.UserFollowing(ctx, id, restrict, offset)
}
func (s *Source) UgoiraMetadata(ctx context.Context, id int64) (*UgoiraMetadataResult, error) {
	if s.useWeb(opUgoiraMetadata) {
		return s.web.UgoiraMetadata(ctx, id)
	}
	return s.app.UgoiraMetadata(ctx, id)
}
func (s *Source) Download(ctx context.Context, rawURL string, dst io.Writer) error {
	if s.useWeb(opDownload) {
		return s.webResource.Download(ctx, rawURL, dst)
	}
	return s.appResource.Download(ctx, rawURL, dst)
}
