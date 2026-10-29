package pixiv

import (
	"context"
	"errors"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// ArtworkRequest selects one artwork by its stable ID.
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

// ArtworkRankingRequest lists an artwork ranking.
type ArtworkRankingRequest struct {
	// Mode defaults to RankingModeDay.
	Mode RankingMode
	// Date is YYYY-MM-DD; empty asks upstream for the current ranking.
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
	path:     api.PathIllustRelated,
	keys:     []string{api.ParamOffset},
	query:    []string{api.ParamIllustID},
	prefixes: []string{"seed_illust_ids[", "viewed["},
}

var recommendedContinuation = continuationSpec{
	path: api.PathIllustRecommended,
	keys: []string{
		api.ParamOffset, api.ParamMinRecentIllusts, api.ParamMaxRecommend,
		api.ParamIncludeRanking, api.ParamIncludePrivacy,
	},
	allowZero: true,
	ignored:   []string{"viewed["},
}

var rankingContinuation = continuationSpec{
	path:  api.PathIllustRanking,
	keys:  []string{api.ParamOffset},
	query: []string{api.ParamMode, api.ParamDate},
}

var seriesContinuation = continuationSpec{
	path:  api.PathIllustSeries,
	keys:  []string{api.ParamOffset, api.ParamLastOrder},
	query: []string{api.ParamIllustSeriesID},
}

var followingContinuation = continuationSpec{
	path:  api.PathIllustFollow,
	keys:  []string{api.ParamOffset},
	query: []string{api.ParamRestrict},
}

var latestContinuation = continuationSpec{
	path:  api.PathIllustNew,
	keys:  []string{api.ParamMaxIllustID, api.ParamOffset},
	query: []string{api.ParamContentType, api.ParamFilter},
}

var userArtworksContinuation = continuationSpec{
	path:  api.PathUserIllusts,
	keys:  []string{api.ParamOffset},
	query: []string{api.ParamUserID, api.ParamType},
}

var myPixivContinuation = continuationSpec{
	path: api.PathIllustMyPixiv,
	keys: []string{api.ParamOffset},
}

type artworkDetailWire struct {
	Illust *artworkWire `json:"illust"`
}

type artworkSeriesWire struct {
	SeriesDetail *struct {
		User userWire `json:"user"`
	} `json:"illust_series_detail"`
	Illusts api.RequiredList[artworkWire] `json:"illusts"`
	NextURL *string                       `json:"next_url"`
}

type trendingArtworksWire struct {
	Tags api.RequiredList[struct {
		Tag            string                          `json:"tag"`
		TranslatedName string                          `json:"translated_name"`
		Illust         api.RequiredObject[artworkWire] `json:"illust"`
	}] `json:"trend_tags"`
}

// relatedParams normalizes the indexed array members of a related next_url into
// slice keys that can be replayed. Offset-form continuations pass through
// unchanged.
func relatedParams(rawURL *string) (url.Values, bool, error) {
	values, hasNext, err := relatedContinuation.params(rawURL)
	if err != nil || !hasNext {
		return nil, hasNext, err
	}
	if rawOffset := values.Get(api.ParamOffset); rawOffset != "" {
		value, parseErr := strconv.ParseInt(rawOffset, 10, 64)
		if parseErr != nil || value <= 0 || int64(int(value)) != value {
			return nil, false, api.MalformedResponse()
		}
		return values, true, nil
	}
	seedIDs, ok := indexedContinuationValues(values, "seed_illust_ids[")
	if !ok {
		return nil, false, api.MalformedResponse()
	}
	viewed, ok := indexedContinuationValues(values, "viewed[")
	if !ok {
		return nil, false, api.MalformedResponse()
	}
	return url.Values{
		api.ParamIllustID:      append([]string(nil), values[api.ParamIllustID]...),
		api.ParamSeedIllustIDs: seedIDs,
		api.ParamViewed:        viewed,
	}, true, nil
}

// indexedContinuationValues restores parameters such as viewed[0]=… into an
// ordered slice. Indices must start at zero, stay contiguous, and carry positive
// values.
func indexedContinuationValues(params url.Values, prefix string) ([]string, bool) {
	indexed := make(map[int]string)
	for key, entries := range params {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		if len(entries) != 1 || entries[0] == "" || !strings.HasSuffix(key, "]") {
			return nil, false
		}
		indexText := strings.TrimSuffix(strings.TrimPrefix(key, prefix), "]")
		index, err := strconv.Atoi(indexText)
		if err != nil || index < 0 {
			return nil, false
		}
		value, err := strconv.ParseInt(entries[0], 10, 64)
		if err != nil || value <= 0 {
			return nil, false
		}
		indexed[index] = entries[0]
	}
	if len(indexed) == 0 {
		return nil, false
	}
	values := make([]string, len(indexed))
	for index := range values {
		value, exists := indexed[index]
		if !exists {
			return nil, false
		}
		values[index] = value
	}
	return values, true
}

func artworkKind(raw string) (ArtworkKind, string) {
	switch raw {
	case "illust":
		return ArtworkKindIllustration, raw
	case "manga":
		return ArtworkKindManga, raw
	case "ugoira":
		return ArtworkKindUgoira, raw
	default:
		return ArtworkKindUnknown, raw
	}
}

func (c *Client) artworkPage(op string, query url.Values, key string, values []artworkWire, nextValue int64, hasNext bool) (sdk.Page[Artwork], error) {
	items, err := c.mapArtworks(values)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	next, err := c.buildCursor(op, query, key, nextValue, hasNext)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	return sdk.Page[Artwork]{Items: items, Next: next}, nil
}

// artworkParamsPage maps artworks and builds a cursor from a multi-parameter
// continuation, as recommended endpoints supply.
func (c *Client) artworkParamsPage(op string, query url.Values, values []artworkWire, nextParams url.Values, hasNext bool) (sdk.Page[Artwork], error) {
	items, err := c.mapArtworks(values)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	var next sdk.Cursor
	if hasNext {
		built, err := c.buildContinuationCursor(op, query, continuationEnvelope{Params: nextParams})
		if err != nil {
			return sdk.Page[Artwork]{}, err
		}
		next = built
	}
	return sdk.Page[Artwork]{Items: items, Next: next}, nil
}

// Artwork returns one artwork by its stable ID, including every image page.
func (c *Client) Artwork(ctx context.Context, request ArtworkRequest) (Artwork, error) {
	ctx = logging.WithOperation(ctx, "Artwork")
	if request.ArtworkID <= 0 {
		return Artwork{}, newError("Artwork", sdk.InvalidArgument, "artwork ID must be positive")
	}
	detail, err := c.fetchArtworkDetail(ctx, "Artwork", request.ArtworkID)
	if err != nil {
		return Artwork{}, err
	}
	result, err := c.mapArtwork(*detail.Illust)
	if err != nil {
		return Artwork{}, err
	}
	result.Pages, err = c.mapArtworkPages(*detail.Illust)
	if err != nil {
		return Artwork{}, err
	}
	return result, nil
}

// ArtworkPages returns every image page of one artwork as usable resources.
func (c *Client) ArtworkPages(ctx context.Context, request ArtworkPagesRequest) ([]ArtworkPage, error) {
	ctx = logging.WithOperation(ctx, "ArtworkPages")
	if request.ArtworkID <= 0 {
		return nil, newError("ArtworkPages", sdk.InvalidArgument, "artwork ID must be positive")
	}
	detail, err := c.fetchArtworkDetail(ctx, "ArtworkPages", request.ArtworkID)
	if err != nil {
		return nil, err
	}
	return c.mapArtworkPages(*detail.Illust)
}

func (c *Client) fetchArtworkDetail(ctx context.Context, operation string, artworkID int64) (artworkDetailWire, error) {
	var raw artworkDetailWire
	query := url.Values{api.ParamIllustID: {itoa(artworkID)}}
	if err := c.app.GetJSON(ctx, api.PathIllustDetail, query, &raw); err != nil {
		return artworkDetailWire{}, classifyAppError(err, operation)
	}
	if raw.Illust == nil || raw.Illust.ID <= 0 {
		return artworkDetailWire{}, newError(operation, sdk.MalformedUpstreamResponse, "")
	}
	return raw, nil
}

// RelatedArtworks lists artworks related to one artwork.
func (c *Client) RelatedArtworks(ctx context.Context, request RelatedArtworksRequest) (sdk.Page[Artwork], error) {
	ctx = logging.WithOperation(ctx, "RelatedArtworks")
	if request.ArtworkID <= 0 {
		return sdk.Page[Artwork]{}, newError("RelatedArtworks", sdk.InvalidArgument, "artwork ID must be positive")
	}
	base := url.Values{api.ParamIllustID: {itoa(request.ArtworkID)}}
	params, err := c.continuationParams("RelatedArtworks", base, request.Cursor)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	query := base
	if params != nil {
		if params.Get(api.ParamIllustID) != itoa(request.ArtworkID) {
			return sdk.Page[Artwork]{}, newError("RelatedArtworks", sdk.InvalidCursor, "cursor continuation artwork mismatch")
		}
		query = params
	}

	var raw artworkListWire
	if err := c.app.GetJSON(ctx, api.PathIllustRelated, query, &raw); err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "RelatedArtworks")
	}
	items, err := artworkItems("RelatedArtworks", raw.Illusts, false)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	nextParams, hasNext, err := relatedParams(raw.NextURL)
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "RelatedArtworks")
	}
	return c.artworkParamsPage("RelatedArtworks", base, items, nextParams, hasNext)
}

// ArtworkSeries lists artworks within one illustration series.
func (c *Client) ArtworkSeries(ctx context.Context, request ArtworkSeriesRequest) (sdk.Page[Artwork], error) {
	ctx = logging.WithOperation(ctx, "ArtworkSeries")
	if request.SeriesID <= 0 {
		return sdk.Page[Artwork]{}, newError("ArtworkSeries", sdk.InvalidArgument, "series ID must be positive")
	}
	base := url.Values{api.ParamIllustSeriesID: {itoa(request.SeriesID)}}
	query := cloneValues(base)
	key, value, err := c.continuationKeyValue("ArtworkSeries", base, request.Cursor, seriesContinuation)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	if value > 0 {
		query.Set(key, itoa(value))
	}

	var raw artworkSeriesWire
	if err := c.app.GetJSON(ctx, api.PathIllustSeries, query, &raw); err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "ArtworkSeries")
	}
	if raw.SeriesDetail == nil || raw.SeriesDetail.User.ID <= 0 {
		return sdk.Page[Artwork]{}, newError("ArtworkSeries", sdk.MalformedUpstreamResponse, "")
	}
	items, err := artworkItems("ArtworkSeries", raw.Illusts, true)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	nextKey, nextValue, hasNext, err := seriesContinuation.keyValue(raw.NextURL)
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "ArtworkSeries")
	}
	return c.artworkPage("ArtworkSeries", base, nextKey, items, nextValue, hasNext)
}

// ArtworkRanking lists the current artwork ranking.
func (c *Client) ArtworkRanking(ctx context.Context, request ArtworkRankingRequest) (sdk.Page[Artwork], error) {
	ctx = logging.WithOperation(ctx, "ArtworkRanking")
	if request.Mode == "" {
		request.Mode = RankingModeDay
	}
	if err := validateRankingMode("ArtworkRanking", request.Mode); err != nil {
		return sdk.Page[Artwork]{}, err
	}
	if err := validateDate("ArtworkRanking", "date", request.Date); err != nil {
		return sdk.Page[Artwork]{}, err
	}
	base := url.Values{api.ParamMode: {string(request.Mode)}}
	if request.Date != "" {
		base.Set(api.ParamDate, request.Date)
	}
	offset, err := c.continuationPositiveOffset("ArtworkRanking", base, request.Cursor)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	query := cloneValues(base)
	if offset > 0 {
		query.Set(api.ParamOffset, itoa(int64(offset)))
	}

	var raw artworkListWire
	if err := c.app.GetJSON(ctx, api.PathIllustRanking, query, &raw); err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "ArtworkRanking")
	}
	items, err := artworkItems("ArtworkRanking", raw.Illusts, false)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	next, hasNext, err := rankingContinuation.offset(raw.NextURL)
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "ArtworkRanking")
	}
	return c.artworkPage("ArtworkRanking", base, api.ParamOffset, items, int64(next), hasNext)
}

// RecommendedArtworks lists recommended artworks. Upstream expresses the next
// page as a multi-parameter next_url; the cursor keeps the offset (including the
// offset=0 second page) and bookmark cursor parameters as a unit, drops the
// viewed index arrays, and never stores the raw next_url.
func (c *Client) RecommendedArtworks(ctx context.Context, request RecommendedArtworksRequest) (sdk.Page[Artwork], error) {
	ctx = logging.WithOperation(ctx, "RecommendedArtworks")
	base := url.Values{}
	params, err := c.continuationParams("RecommendedArtworks", base, request.Cursor)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}

	var raw artworkListWire
	if err := c.app.GetJSON(ctx, api.PathIllustRecommended, params, &raw); err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "RecommendedArtworks")
	}
	items, err := artworkItems("RecommendedArtworks", raw.Illusts, false)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	nextParams, hasNext, err := recommendedContinuation.params(raw.NextURL)
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "RecommendedArtworks")
	}
	return c.artworkParamsPage("RecommendedArtworks", base, items, nextParams, hasNext)
}

// FollowingArtworks lists artworks by followed users.
func (c *Client) FollowingArtworks(ctx context.Context, request FollowingArtworksRequest) (sdk.Page[Artwork], error) {
	ctx = logging.WithOperation(ctx, "FollowingArtworks")
	restrict, err := normalizeFollowingRestrict("FollowingArtworks", request.Restrict)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	base := url.Values{api.ParamRestrict: {string(restrict)}}
	offset, err := c.continuationPositiveOffset("FollowingArtworks", base, request.Cursor)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	query := cloneValues(base)
	if offset > 0 {
		query.Set(api.ParamOffset, itoa(int64(offset)))
	}

	var raw artworkListWire
	if err := c.app.GetJSON(ctx, api.PathIllustFollow, query, &raw); err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "FollowingArtworks")
	}
	items, err := artworkItems("FollowingArtworks", raw.Illusts, false)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	next, hasNext, err := followingContinuation.offset(raw.NextURL)
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "FollowingArtworks")
	}
	return c.artworkPage("FollowingArtworks", base, api.ParamOffset, items, int64(next), hasNext)
}

// LatestArtworks lists the newest artworks.
func (c *Client) LatestArtworks(ctx context.Context, request LatestArtworksRequest) (sdk.Page[Artwork], error) {
	ctx = logging.WithOperation(ctx, "LatestArtworks")
	contentType, err := normalizeLatestArtworkContentType("LatestArtworks", request.ContentType)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	// Bind the resolved content type into the cursor digest so a continuation
	// produced for one feed (e.g. illust) cannot be replayed against another
	// (e.g. manga), which would resume at an offset for the wrong result set.
	base := url.Values{
		api.ParamContentType: {contentType},
		api.ParamFilter:      {"for_android"},
	}
	key, value, err := c.continuationKeyValue("LatestArtworks", base, request.Cursor, latestContinuation)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	query := cloneValues(base)
	if value > 0 {
		query.Set(key, itoa(value))
	}

	var raw artworkListWire
	if err := c.app.GetJSON(ctx, api.PathIllustNew, query, &raw); err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "LatestArtworks")
	}
	items, err := artworkItems("LatestArtworks", raw.Illusts, false)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	nextKey, nextValue, hasNext, err := latestContinuation.keyValue(raw.NextURL)
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "LatestArtworks")
	}
	return c.artworkPage("LatestArtworks", base, nextKey, items, nextValue, hasNext)
}

// UserArtworks lists one user's artworks.
func (c *Client) UserArtworks(ctx context.Context, request UserArtworksRequest) (sdk.Page[Artwork], error) {
	ctx = logging.WithOperation(ctx, "UserArtworks")
	if request.UserID <= 0 {
		return sdk.Page[Artwork]{}, newError("UserArtworks", sdk.InvalidArgument, "user ID must be positive")
	}
	kind, err := normalizeUserArtworkKind("UserArtworks", request.Kind)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	base := url.Values{api.ParamUserID: {itoa(request.UserID)}, api.ParamType: {kind}}
	offset, err := c.continuationPositiveOffset("UserArtworks", base, request.Cursor)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	query := cloneValues(base)
	if offset > 0 {
		query.Set(api.ParamOffset, itoa(int64(offset)))
	}

	var raw artworkListWire
	if err := c.app.GetJSON(ctx, api.PathUserIllusts, query, &raw); err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "UserArtworks")
	}
	items, err := artworkItems("UserArtworks", raw.Illusts, false)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	next, hasNext, err := userArtworksContinuation.offset(raw.NextURL)
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "UserArtworks")
	}
	return c.artworkPage("UserArtworks", base, api.ParamOffset, items, int64(next), hasNext)
}

// MyPixivArtworks lists artworks from the current user's MyPixiv feed.
func (c *Client) MyPixivArtworks(ctx context.Context, request MyPixivArtworksRequest) (sdk.Page[Artwork], error) {
	ctx = logging.WithOperation(ctx, "MyPixivArtworks")
	base := url.Values{}
	offset, err := c.continuationPositiveOffset("MyPixivArtworks", base, request.Cursor)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	query := cloneValues(base)
	if offset > 0 {
		query.Set(api.ParamOffset, itoa(int64(offset)))
	}

	var raw artworkListWire
	if err := c.app.GetJSON(ctx, api.PathIllustMyPixiv, query, &raw); err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "MyPixivArtworks")
	}
	items, err := artworkItems("MyPixivArtworks", raw.Illusts, true)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	next, hasNext, err := myPixivContinuation.offset(raw.NextURL)
	if err != nil {
		return sdk.Page[Artwork]{}, classifyAppError(err, "MyPixivArtworks")
	}
	return c.artworkPage("MyPixivArtworks", base, api.ParamOffset, items, int64(next), hasNext)
}

// TrendingArtworkTags lists currently trending artwork tags.
func (c *Client) TrendingArtworkTags(ctx context.Context, request TrendingArtworkTagsRequest) ([]TrendingTag, error) {
	ctx = logging.WithOperation(ctx, "TrendingArtworkTags")
	var raw trendingArtworksWire
	if err := c.app.GetJSON(ctx, api.PathTrendingTagsIllust, nil, &raw); err != nil {
		return nil, classifyAppError(err, "TrendingArtworkTags")
	}
	if !raw.Tags.Present || !raw.Tags.Valid {
		return nil, newError("TrendingArtworkTags", sdk.MalformedUpstreamResponse, "")
	}
	items := make([]TrendingTag, len(raw.Tags.Items))
	for index, value := range raw.Tags.Items {
		if value.Tag == "" || !value.Illust.Present || !value.Illust.Valid || value.Illust.Value.ID <= 0 {
			return nil, newError("TrendingArtworkTags", sdk.MalformedUpstreamResponse, "")
		}
		mapped, err := c.mapArtwork(value.Illust.Value)
		if err != nil {
			return nil, err
		}
		items[index] = TrendingTag{Tag: value.Tag, TranslatedName: value.TranslatedName, Artwork: mapped}
	}
	return items, nil
}

// UgoiraMetadata returns the playable metadata of a ugoira artwork. The artwork
// must be a ugoira; other kinds return InvalidArgument.
func (c *Client) UgoiraMetadata(ctx context.Context, request UgoiraMetadataRequest) (UgoiraMetadata, error) {
	ctx = logging.WithOperation(ctx, "UgoiraMetadata")
	if request.ArtworkID <= 0 {
		return UgoiraMetadata{}, newError("UgoiraMetadata", sdk.InvalidArgument, "artwork ID must be positive")
	}
	var raw ugoiraResponseWire
	query := url.Values{api.ParamIllustID: {itoa(request.ArtworkID)}}
	if err := c.app.GetJSON(ctx, api.PathUgoiraMetadata, query, &raw); err != nil {
		return UgoiraMetadata{}, classifyAppError(err, "UgoiraMetadata")
	}
	if !raw.Metadata.Present || !raw.Metadata.Valid {
		return UgoiraMetadata{}, newError("UgoiraMetadata", sdk.MalformedUpstreamResponse, "")
	}
	metadata, err := c.mapUgoiraMetadata(request.ArtworkID, raw.Metadata.Value)
	if err != nil {
		// Mapping already returned a public SDK error such as ResourceForbidden.
		// Reclassifying it as an upstream failure would discard the original reason.
		var sdkErr *sdk.Error
		if errors.As(err, &sdkErr) {
			return UgoiraMetadata{}, err
		}
		return UgoiraMetadata{}, classifyAppError(err, "UgoiraMetadata")
	}
	return metadata, nil
}

// artworkItems checks the required fields of an artwork list and returns the
// wire items unchanged.
func artworkItems(operation string, list api.RequiredList[artworkWire], requireUser bool) ([]artworkWire, error) {
	if !list.Present || !list.Valid {
		return nil, newError(operation, sdk.MalformedUpstreamResponse, "")
	}
	for _, item := range list.Items {
		if item.ID <= 0 || requireUser && item.User.ID <= 0 {
			return nil, newError(operation, sdk.MalformedUpstreamResponse, "")
		}
	}
	return list.Items, nil
}

// mapArtworkPages builds page resources for an artwork detail response; list
// responses do not carry pages.
func (c *Client) mapArtworkPages(value artworkWire) ([]ArtworkPage, error) {
	if len(value.MetaPages) > 0 {
		pages := make([]ArtworkPage, 0, len(value.MetaPages))
		for index, page := range value.MetaPages {
			imageURL := firstImageURL(page.ImageURLs)
			if imageURL == "" {
				return nil, newError("ArtworkPages", sdk.MalformedUpstreamResponse, "page has no image URL")
			}
			resource, err := c.newResource("artwork", value.ID, index, "original", imageURL)
			if err != nil {
				return nil, err
			}
			pages = append(pages, ArtworkPage{
				PageIndex: index,
				Image: ImageResource{
					Resource: resource,
					Variant:  "original",
					Width:    page.Width,
					Height:   page.Height,
				},
				Width:  page.Width,
				Height: page.Height,
			})
		}
		return pages, nil
	}
	if value.MetaSinglePage.OriginalImageURL == "" {
		return []ArtworkPage{}, nil
	}
	resource, err := c.newResource("artwork", value.ID, 0, "original", value.MetaSinglePage.OriginalImageURL)
	if err != nil {
		return nil, err
	}
	return []ArtworkPage{{
		PageIndex: 0,
		Image: ImageResource{
			Resource: resource,
			Variant:  "original",
			Width:    value.Width,
			Height:   value.Height,
		},
		Width:  value.Width,
		Height: value.Height,
	}}, nil
}

// mapUgoiraMetadata validates and maps the archives and frames needed to play a
// ugoira animation.
func (c *Client) mapUgoiraMetadata(artworkID int64, value ugoiraMetadataWire) (UgoiraMetadata, error) {
	if !value.ZipURLs.Present || !value.ZipURLs.Valid ||
		(value.ZipURLs.Value.Medium == "" && value.ZipURLs.Value.Original == "") ||
		!value.Frames.Present || !value.Frames.Valid || len(value.Frames.Items) == 0 {
		return UgoiraMetadata{}, api.MalformedResponse()
	}
	archives := make([]UgoiraArchive, 0, 2)
	for _, candidate := range []struct {
		quality UgoiraQuality
		url     string
	}{
		{UgoiraQualityMedium, value.ZipURLs.Value.Medium},
		{UgoiraQualityOriginal, value.ZipURLs.Value.Original},
	} {
		if candidate.url == "" {
			continue
		}
		resource, err := c.newResource("ugoira_archive", artworkID, -1, string(candidate.quality), candidate.url)
		if err != nil {
			return UgoiraMetadata{}, err
		}
		archives = append(archives, UgoiraArchive{Quality: candidate.quality, Resource: resource})
	}
	frames := make([]UgoiraFrame, 0, len(value.Frames.Items))
	seenFile := map[string]bool{}
	for _, frame := range value.Frames.Items {
		if !safeArtworkArchiveFilename(frame.File) || seenFile[frame.File] {
			return UgoiraMetadata{}, api.MalformedResponse()
		}
		seenFile[frame.File] = true
		frames = append(frames, UgoiraFrame{Filename: frame.File, DelayMilliseconds: frame.Delay})
	}
	return UgoiraMetadata{ArtworkID: artworkID, Archives: archives, Frames: frames}, nil
}

// safeArtworkArchiveFilename rejects frame names that would reach archive
// extraction or a filesystem destination: absolute paths, path traversal, NUL,
// and empty segments.
func safeArtworkArchiveFilename(name string) bool {
	if name == "" || strings.ContainsAny(name, "\\\x00") {
		return false
	}
	if path.IsAbs(name) || len(name) >= 2 && name[1] == ':' {
		return false
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}
