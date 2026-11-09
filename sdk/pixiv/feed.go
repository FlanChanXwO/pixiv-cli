package pixiv

import (
	"context"
	"net/url"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// ArtworkRankingOptions selects a ranking and optional YYYY-MM-DD date.
// Zero Mode selects the daily ranking.
type ArtworkRankingOptions struct {
	Mode   RankingMode
	Date   string
	Cursor sdk.Cursor
}

// NovelRankingOptions selects a novel ranking; zero Mode selects the daily ranking.
type NovelRankingOptions struct {
	Mode   RankingMode
	Cursor sdk.Cursor
}

// RecommendedArtworksOptions resumes the same account's artwork recommendations.
type RecommendedArtworksOptions struct{ Cursor sdk.Cursor }

// RecommendedNovelsOptions resumes the same account's novel recommendations.
type RecommendedNovelsOptions struct{ Cursor sdk.Cursor }

// FollowingOptions selects public or private follows; zero Restrict means public.
type FollowingOptions struct {
	Restrict Restrict
	Cursor   sdk.Cursor
}

// LatestArtworksOptions selects illustrations or manga; zero Category means illustrations.
type LatestArtworksOptions struct {
	Category ArtworkCategory
	Cursor   sdk.Cursor
}

// LatestNovelsOptions resumes the newest novels.
type LatestNovelsOptions struct{ Cursor sdk.Cursor }

// ArtworkRanking lists artworks in the selected ranking.
func (c *Client) ArtworkRanking(ctx context.Context, options ArtworkRankingOptions) (sdk.Page[*Artwork], error) {
	mode, err := rankingMode(options.Mode)
	if err != nil {
		return sdk.Page[*Artwork]{}, err
	}
	if !validDate(options.Date) {
		return sdk.Page[*Artwork]{}, newError("ArtworkRanking", sdk.InvalidArgument, "invalid ranking date", nil)
	}
	query := url.Values{api.ParamMode: {string(mode)}}
	if options.Date != "" {
		query.Set(api.ParamDate, options.Date)
	}
	return c.artworks(ctx, api.PathIllustRanking, query, options.Cursor)
}

// NovelRanking lists novels in the selected ranking with the App filter.
func (c *Client) NovelRanking(ctx context.Context, options NovelRankingOptions) (sdk.Page[*Novel], error) {
	mode, err := rankingMode(options.Mode)
	if err != nil {
		return sdk.Page[*Novel]{}, err
	}
	return c.novels(ctx, api.PathNovelRanking, url.Values{api.ParamMode: {string(mode)}, api.ParamFilter: {"for_android"}}, options.Cursor)
}

// RecommendedArtworks lists recommendations with their multi-parameter continuation.
func (c *Client) RecommendedArtworks(ctx context.Context, options RecommendedArtworksOptions) (sdk.Page[*Artwork], error) {
	return c.artworks(ctx, api.PathIllustRecommended, url.Values{}, options.Cursor)
}

// RecommendedNovels lists recommendations with their multi-parameter continuation.
func (c *Client) RecommendedNovels(ctx context.Context, options RecommendedNovelsOptions) (sdk.Page[*Novel], error) {
	return c.novels(ctx, api.PathNovelRecommended, url.Values{}, options.Cursor)
}

// FollowingArtworks lists artworks by the account's followed users.
func (c *Client) FollowingArtworks(ctx context.Context, options FollowingOptions) (sdk.Page[*Artwork], error) {
	restrict, err := normalizeRestrict(options.Restrict)
	if err != nil {
		return sdk.Page[*Artwork]{}, err
	}
	return c.artworks(ctx, api.PathIllustFollow, url.Values{api.ParamRestrict: {string(restrict)}}, options.Cursor)
}

// FollowingNovels lists novels by the account's followed users.
func (c *Client) FollowingNovels(ctx context.Context, options FollowingOptions) (sdk.Page[*Novel], error) {
	restrict, err := normalizeRestrict(options.Restrict)
	if err != nil {
		return sdk.Page[*Novel]{}, err
	}
	return c.novels(ctx, api.PathNovelFollow, url.Values{api.ParamRestrict: {string(restrict)}}, options.Cursor)
}

// LatestArtworks lists the newest illustrations or manga.
func (c *Client) LatestArtworks(ctx context.Context, options LatestArtworksOptions) (sdk.Page[*Artwork], error) {
	category := options.Category
	if category == "" {
		category = ArtworkCategoryIllust
	}
	if category != ArtworkCategoryIllust && category != ArtworkCategoryManga {
		return sdk.Page[*Artwork]{}, newError("LatestArtworks", sdk.InvalidArgument, "unsupported latest artwork category", nil)
	}
	return c.artworks(ctx, api.PathIllustNew, url.Values{api.ParamContentType: {string(category)}, api.ParamFilter: {"for_android"}}, options.Cursor)
}

// LatestNovels lists the newest novels with the App filter.
func (c *Client) LatestNovels(ctx context.Context, options LatestNovelsOptions) (sdk.Page[*Novel], error) {
	return c.novels(ctx, api.PathNovelNew, url.Values{api.ParamFilter: {"for_android"}}, options.Cursor)
}
