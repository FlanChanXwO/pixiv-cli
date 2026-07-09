package pixiv

import (
	"context"
	"io"
	"strings"
	"sync"
)

type AppAPI interface {
	Refresh(context.Context) error
	SetRefreshToken(string)
	RefreshTokenValue() string
	UserID() int64
	UserName() string
	IsAuthenticated() bool
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
	Download(context.Context, string, io.Writer) error
}

type WebAPI interface {
	SearchIllust(context.Context, string, string, string, string, int) (*IllustList, error)
	IllustDetail(context.Context, int64) (*IllustDetail, error)
	IllustRanking(context.Context, string, string, int) (*IllustList, error)
	SearchUser(context.Context, string, int) (*UserPreviewList, error)
	UgoiraMetadata(context.Context, int64) (*UgoiraMetadataResult, error)
	Download(context.Context, string, io.Writer) error
}

type SourcePolicy struct {
	RefreshToken       string
	WebFallbackEnabled bool
}

type Source struct {
	app    AppAPI
	web    WebAPI
	mu     sync.RWMutex
	useWeb bool
}

func NewSourceFromClients(app AppAPI, web WebAPI, cfg SourcePolicy) *Source {
	return &Source{
		app:    app,
		web:    web,
		useWeb: strings.TrimSpace(cfg.RefreshToken) == "" && cfg.WebFallbackEnabled && web != nil,
	}
}

func (s *Source) Refresh(ctx context.Context) error {
	return s.app.Refresh(ctx)
}

func (s *Source) SetRefreshToken(token string) {
	s.app.SetRefreshToken(token)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.useWeb = false
}

func (s *Source) RefreshTokenValue() string {
	return s.app.RefreshTokenValue()
}

func (s *Source) UserID() int64 {
	return s.app.UserID()
}

func (s *Source) UserName() string {
	return s.app.UserName()
}

func (s *Source) IsAuthenticated() bool {
	return s.app.IsAuthenticated()
}

func (s *Source) SearchIllust(ctx context.Context, word, target, sort, duration string, offset int) (*IllustList, error) {
	if s.useWebFallback() {
		return s.web.SearchIllust(ctx, word, target, sort, duration, offset)
	}
	return s.app.SearchIllust(ctx, word, target, sort, duration, offset)
}

func (s *Source) IllustDetail(ctx context.Context, id int64) (*IllustDetail, error) {
	if s.useWebFallback() {
		return s.web.IllustDetail(ctx, id)
	}
	return s.app.IllustDetail(ctx, id)
}

func (s *Source) IllustRelated(ctx context.Context, id int64, offset int) (*IllustList, error) {
	return s.app.IllustRelated(ctx, id, offset)
}

func (s *Source) IllustRanking(ctx context.Context, mode, date string, offset int) (*IllustList, error) {
	if s.useWebFallback() {
		return s.web.IllustRanking(ctx, mode, date, offset)
	}
	return s.app.IllustRanking(ctx, mode, date, offset)
}

func (s *Source) SearchUser(ctx context.Context, word string, offset int) (*UserPreviewList, error) {
	if s.useWebFallback() {
		return s.web.SearchUser(ctx, word, offset)
	}
	return s.app.SearchUser(ctx, word, offset)
}

func (s *Source) UserDetail(ctx context.Context, userID int64) (*User, error) {
	return s.app.UserDetail(ctx, userID)
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

func (s *Source) UserBookmarks(ctx context.Context, userID int64, restrict, tag string, maxBookmarkID int64) (*IllustList, error) {
	return s.app.UserBookmarks(ctx, userID, restrict, tag, maxBookmarkID)
}

func (s *Source) UserFollowing(ctx context.Context, userID int64, restrict string, offset int) (*UserPreviewList, error) {
	return s.app.UserFollowing(ctx, userID, restrict, offset)
}

func (s *Source) UgoiraMetadata(ctx context.Context, id int64) (*UgoiraMetadataResult, error) {
	if s.useWebFallback() {
		return s.web.UgoiraMetadata(ctx, id)
	}
	return s.app.UgoiraMetadata(ctx, id)
}

func (s *Source) Download(ctx context.Context, rawURL string, dst io.Writer) error {
	if s.useWebFallback() {
		return s.web.Download(ctx, rawURL, dst)
	}
	return s.app.Download(ctx, rawURL, dst)
}

func (s *Source) useWebFallback() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.useWeb
}
