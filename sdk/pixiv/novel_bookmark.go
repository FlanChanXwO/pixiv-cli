package pixiv

import (
	"context"
	"net/url"
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// NovelBookmarkDetail records visibility and registered tags; empty Restrict means unbookmarked.
type NovelBookmarkDetail struct {
	Restrict Restrict `json:"restrict"`
	Tags     []string `json:"tags"`
}

// NovelBookmarksOptions selects a user's novel bookmarks; zero visibility is sent unchanged.
type NovelBookmarksOptions struct {
	Restrict Restrict
	Tag      string
	Cursor   sdk.Cursor
}

// NovelBookmarkTagsOptions selects visibility; Cursor must be zero because the endpoint has no continuation.
type NovelBookmarkTagsOptions struct {
	Restrict Restrict
	Cursor   sdk.Cursor
}

// Bookmark reads the current account's bookmark and its registered tags.
func (r *NovelRef) Bookmark(ctx context.Context) (NovelBookmarkDetail, error) {
	if err := r.bound("Novel.Bookmark"); err != nil {
		return NovelBookmarkDetail{}, err
	}
	restrict, tags, err := r.client.bookmarkDetail(ctx, "Novel.Bookmark", api.PathNovelBookmarkDetail, api.ParamNovelID, r.ID)
	return NovelBookmarkDetail{Restrict: restrict, Tags: tags}, err
}

// AddBookmark bookmarks this novel for the client account, sending the mutation once.
func (r *NovelRef) AddBookmark(ctx context.Context, options BookmarkOptions) error {
	if err := r.bound("Novel.AddBookmark"); err != nil {
		return err
	}
	restrict, err := normalizeRestrict(options.Restrict)
	if err != nil {
		return err
	}
	form := url.Values{api.ParamNovelID: {strconv.FormatInt(r.ID, 10)}, api.ParamRestrict: {string(restrict)}}
	for _, tag := range options.Tags {
		form.Add(api.ParamTags, tag)
	}
	return r.client.postForm(ctx, api.PathNovelBookmarkAdd, form)
}

// RemoveBookmark removes this novel from the current account's bookmarks.
func (r *NovelRef) RemoveBookmark(ctx context.Context) error {
	if err := r.bound("Novel.RemoveBookmark"); err != nil {
		return err
	}
	return r.client.postForm(ctx, api.PathNovelBookmarkDelete, url.Values{api.ParamNovelID: {strconv.FormatInt(r.ID, 10)}})
}

// NovelBookmarks lists this user's novel bookmarks with query-bound continuation.
func (r *UserRef) NovelBookmarks(ctx context.Context, options NovelBookmarksOptions) (sdk.Page[*Novel], error) {
	if err := r.bound("User.NovelBookmarks"); err != nil {
		return sdk.Page[*Novel]{}, err
	}
	if err := validateRestrict(options.Restrict); err != nil {
		return sdk.Page[*Novel]{}, err
	}
	base := url.Values{api.ParamUserID: {strconv.FormatInt(r.ID, 10)}, api.ParamRestrict: {string(options.Restrict)}}
	if options.Tag != "" {
		base.Set(api.ParamTag, options.Tag)
	}
	return r.client.novels(ctx, api.PathUserNovelBookmarks, base, options.Cursor)
}

// NovelBookmarkTags lists this user's novel bookmark tags; the endpoint has no continuation.
func (r *UserRef) NovelBookmarkTags(ctx context.Context, options NovelBookmarkTagsOptions) (sdk.Page[BookmarkTag], error) {
	if err := r.bound("User.NovelBookmarkTags"); err != nil {
		return sdk.Page[BookmarkTag]{}, err
	}
	if err := validateRestrict(options.Restrict); err != nil {
		return sdk.Page[BookmarkTag]{}, err
	}
	if !options.Cursor.IsZero() {
		return sdk.Page[BookmarkTag]{}, newError("User.NovelBookmarkTags", sdk.InvalidArgument, "novel bookmark tags have no continuation", nil)
	}
	var raw struct {
		Tags    api.RequiredList[BookmarkTag] `json:"bookmark_tags"`
		NextURL *string                       `json:"next_url"`
	}
	query := url.Values{api.ParamUserID: {strconv.FormatInt(r.ID, 10)}, api.ParamRestrict: {string(options.Restrict)}}
	if err := r.client.getJSON(ctx, api.PathUserNovelBookmarkTags, query, &raw); err != nil {
		return sdk.Page[BookmarkTag]{}, err
	}
	if !raw.Tags.Present || !raw.Tags.Valid || raw.NextURL != nil {
		return sdk.Page[BookmarkTag]{}, newError("User.NovelBookmarkTags", sdk.MalformedUpstreamResponse, "invalid novel bookmark tags", nil)
	}
	for _, tag := range raw.Tags.Items {
		if tag.Name == "" {
			return sdk.Page[BookmarkTag]{}, newError("User.NovelBookmarkTags", sdk.MalformedUpstreamResponse, "empty bookmark tag", nil)
		}
	}
	return sdk.NewPage(raw.Tags.Items, sdk.Cursor{}, nil), nil
}
