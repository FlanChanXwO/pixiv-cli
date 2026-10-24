// Artwork SDK request types and continuation specifications.
// Wire responses and mapping live in artwork_wire.go.

package pixiv

import (
	"github.com/FlanChanXwO/pixiv-cli/internal/pixivapi"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

type ArtworkRequest struct {
	ArtworkID int64
}

// ArtworkPagesRequest selects the image pages of one artwork.
type ArtworkPagesRequest struct {
	ArtworkID int64
}

// RelatedArtworksRequest lists artworks related to one artwork.
type RelatedArtworksRequest struct {
	ArtworkID int64
	Cursor    sdk.Cursor
}

// ArtworkSeriesRequest lists artworks within one illustration series.
type ArtworkSeriesRequest struct {
	SeriesID int64
	Cursor   sdk.Cursor
}

// ArtworkRankingRequest lists the current artwork ranking.
type ArtworkRankingRequest struct {
	Mode   RankingMode
	Date   string
	Cursor sdk.Cursor
}

// RecommendedArtworksRequest lists recommended artworks.
type RecommendedArtworksRequest struct {
	Cursor sdk.Cursor
}

// FollowingArtworksRequest lists artworks by followed users.
type FollowingArtworksRequest struct {
	Restrict Restrict
	Cursor   sdk.Cursor
}

// LatestArtworksRequest lists the newest artworks.
type LatestArtworksRequest struct {
	ContentType SearchContentType
	Cursor      sdk.Cursor
}

// UserArtworksRequest lists one user's artworks.
type UserArtworksRequest struct {
	UserID int64
	Kind   ArtworkKind
	Cursor sdk.Cursor
}

// MyPixivArtworksRequest lists artworks from the current user's MyPixiv feed.
type MyPixivArtworksRequest struct {
	Cursor sdk.Cursor
}

// TrendingArtworkTagsRequest lists currently trending artwork tags.
type TrendingArtworkTagsRequest struct{}

// UgoiraMetadataRequest selects the ugoira metadata of one artwork. The
// artwork must be a ugoira.
type UgoiraMetadataRequest struct {
	ArtworkID int64
}

var relatedContinuation = continuationSpec{
	path:     pixivapi.PathIllustRelated,
	keys:     []string{pixivapi.ParamOffset},
	query:    []string{pixivapi.ParamIllustID},
	prefixes: []string{"seed_illust_ids[", "viewed["},
}

var recommendedContinuation = continuationSpec{
	path: pixivapi.PathIllustRecommended,
	keys: []string{
		pixivapi.ParamOffset, pixivapi.ParamMinRecentIllusts, pixivapi.ParamMaxRecommend,
		pixivapi.ParamIncludeRanking, pixivapi.ParamIncludePrivacy,
	},
	allowZero: true,
	ignored:   []string{"viewed["},
}

var rankingContinuation = continuationSpec{
	path:  pixivapi.PathIllustRanking,
	keys:  []string{pixivapi.ParamOffset},
	query: []string{pixivapi.ParamMode, pixivapi.ParamDate},
}

var seriesContinuation = continuationSpec{
	path:  pixivapi.PathIllustSeries,
	keys:  []string{pixivapi.ParamOffset, pixivapi.ParamLastOrder},
	query: []string{pixivapi.ParamIllustSeriesID},
}

var followingContinuation = continuationSpec{
	path:  pixivapi.PathIllustFollow,
	keys:  []string{pixivapi.ParamOffset},
	query: []string{pixivapi.ParamRestrict},
}

var latestContinuation = continuationSpec{
	path:  pixivapi.PathIllustNew,
	keys:  []string{pixivapi.ParamMaxIllustID, pixivapi.ParamOffset},
	query: []string{pixivapi.ParamContentType, pixivapi.ParamFilter},
}

var userArtworksContinuation = continuationSpec{
	path:  pixivapi.PathUserIllusts,
	keys:  []string{pixivapi.ParamOffset},
	query: []string{pixivapi.ParamUserID, pixivapi.ParamType},
}

var myPixivContinuation = continuationSpec{
	path: pixivapi.PathIllustMyPixiv,
	keys: []string{pixivapi.ParamOffset},
}
