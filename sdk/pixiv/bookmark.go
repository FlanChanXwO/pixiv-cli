package pixiv

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// ArtworkBookmarkDetail records the account's visibility and registered bookmark tags.
// Empty Restrict and Tags represent an unbookmarked artwork, including upstream 404.
type ArtworkBookmarkDetail struct {
	Restrict Restrict `json:"restrict"`
	Tags     []string `json:"tags"`
}

// BookmarkOptions selects visibility and tags when adding a bookmark.
// Zero visibility defaults to public; tags retain caller order and duplicates.
type BookmarkOptions struct {
	Restrict Restrict
	Tags     []string
}

// ArtworkBookmarksOptions selects a user's bookmarks and their continuation.
// Zero Restrict is sent as the empty upstream scope, matching the App endpoint.
type ArtworkBookmarksOptions struct {
	Restrict Restrict
	Tag      string
	Cursor   sdk.Cursor
}

// ArtworkBookmarkTagsOptions selects visibility and continuation for bookmark tags.
// Zero Restrict is sent as the empty upstream scope.
type ArtworkBookmarkTagsOptions struct {
	Restrict Restrict
	Cursor   sdk.Cursor
}

// BookmarkTag names a bookmark tag and its upstream count.
type BookmarkTag struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func validateRestrict(value Restrict) error {
	switch value {
	case "", RestrictPublic, RestrictPrivate:
		return nil
	}
	return newError("restrict", sdk.InvalidArgument, "unsupported visibility", nil)
}

// Bookmark reads this artwork's bookmark for the client account.
// Only registered bookmark tags are returned.
func (r *ArtworkRef) Bookmark(ctx context.Context) (ArtworkBookmarkDetail, error) {
	if err := r.bound("Artwork.Bookmark"); err != nil {
		return ArtworkBookmarkDetail{}, err
	}
	var raw struct {
		Detail *struct {
			IsBookmarked *bool  `json:"is_bookmarked"`
			Restrict     string `json:"restrict"`
			Tags         []struct {
				Name         string `json:"name"`
				IsRegistered bool   `json:"is_registered"`
			} `json:"tags"`
		} `json:"bookmark_detail"`
	}
	result := ArtworkBookmarkDetail{Tags: []string{}}
	if err := r.client.getJSON(ctx, api.PathIllustBookmarkDetail, url.Values{api.ParamIllustID: {strconv.FormatInt(r.ID, 10)}}, &raw); err != nil {
		var failure *sdk.Error
		if errors.As(err, &failure) && failure.HTTPStatus == http.StatusNotFound {
			return result, nil
		}
		return ArtworkBookmarkDetail{}, err
	}
	if raw.Detail == nil || raw.Detail.IsBookmarked != nil && !*raw.Detail.IsBookmarked {
		return result, nil
	}
	result.Restrict = Restrict(raw.Detail.Restrict)
	for _, tag := range raw.Detail.Tags {
		if tag.IsRegistered {
			result.Tags = append(result.Tags, tag.Name)
		}
	}
	return result, nil
}

// AddBookmark adds this artwork to the client account's bookmarks, sending the write once.
func (r *ArtworkRef) AddBookmark(ctx context.Context, options BookmarkOptions) error {
	if err := r.bound("Artwork.AddBookmark"); err != nil {
		return err
	}
	restrict, err := normalizeRestrict(options.Restrict)
	if err != nil {
		return err
	}
	form := url.Values{api.ParamIllustID: {strconv.FormatInt(r.ID, 10)}, api.ParamRestrict: {string(restrict)}}
	for _, tag := range options.Tags {
		form.Add(api.ParamTags, tag)
	}
	return r.client.postForm(ctx, api.PathBookmarkAdd, form)
}

// RemoveBookmark removes this artwork from the client account's bookmarks.
func (r *ArtworkRef) RemoveBookmark(ctx context.Context) error {
	if err := r.bound("Artwork.RemoveBookmark"); err != nil {
		return err
	}
	return r.client.postForm(ctx, api.PathBookmarkDelete, url.Values{api.ParamIllustID: {strconv.FormatInt(r.ID, 10)}})
}

// ArtworkBookmarks lists this user's bookmarked artworks, filtered by visibility and optional tag.
func (r *UserRef) ArtworkBookmarks(ctx context.Context, options ArtworkBookmarksOptions) (sdk.Page[*Artwork], error) {
	if err := r.bound("User.ArtworkBookmarks"); err != nil {
		return sdk.Page[*Artwork]{}, err
	}
	if err := validateRestrict(options.Restrict); err != nil {
		return sdk.Page[*Artwork]{}, err
	}
	base := url.Values{api.ParamUserID: {strconv.FormatInt(r.ID, 10)}, api.ParamRestrict: {string(options.Restrict)}}
	if options.Tag != "" {
		base.Set(api.ParamTag, options.Tag)
	}
	return r.client.artworks(ctx, api.PathUserBookmarksIllust, base, options.Cursor)
}

// ArtworkBookmarkTags lists tags on this user's artwork bookmarks.
func (r *UserRef) ArtworkBookmarkTags(ctx context.Context, options ArtworkBookmarkTagsOptions) (sdk.Page[BookmarkTag], error) {
	if err := r.bound("User.ArtworkBookmarkTags"); err != nil {
		return sdk.Page[BookmarkTag]{}, err
	}
	if err := validateRestrict(options.Restrict); err != nil {
		return sdk.Page[BookmarkTag]{}, err
	}
	base := url.Values{api.ParamUserID: {strconv.FormatInt(r.ID, 10)}, api.ParamRestrict: {string(options.Restrict)}}
	return r.client.bookmarkTags(ctx, base, options.Cursor)
}

func (c *Client) bookmarkTags(ctx context.Context, base url.Values, token sdk.Cursor) (sdk.Page[BookmarkTag], error) {
	query, binding, err := c.pageQuery(ctx, api.PathUserBookmarkTags, base, token)
	if err != nil {
		return sdk.Page[BookmarkTag]{}, err
	}
	var raw struct {
		Tags    api.RequiredList[BookmarkTag] `json:"bookmark_tags"`
		NextURL *string                       `json:"next_url"`
	}
	if err := c.getJSON(ctx, api.PathUserBookmarkTags, query, &raw); err != nil {
		return sdk.Page[BookmarkTag]{}, err
	}
	if !raw.Tags.Present || !raw.Tags.Valid {
		return sdk.Page[BookmarkTag]{}, newError("User.ArtworkBookmarkTags", sdk.MalformedUpstreamResponse, "missing bookmark tags", nil)
	}
	for _, tag := range raw.Tags.Items {
		if tag.Name == "" {
			return sdk.Page[BookmarkTag]{}, newError("User.ArtworkBookmarkTags", sdk.MalformedUpstreamResponse, "empty bookmark tag", nil)
		}
	}
	next, err := nextCursor(raw.NextURL, api.PathUserBookmarkTags, base, binding)
	if err != nil {
		return sdk.Page[BookmarkTag]{}, err
	}
	return sdk.NewPage(raw.Tags.Items, next, func(ctx context.Context) (sdk.Page[BookmarkTag], error) { return c.bookmarkTags(ctx, base, next) }), nil
}
