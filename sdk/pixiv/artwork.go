// artwork 领域：详情、相关、排行、推荐、时间线、系列、趋势、用户作品与动图元数据。
// 请求类型、响应结构与映射、continuation Spec 都在本文件；共用映射在 response.go，游标在 cursor.go。

package pixiv

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixivapi"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// relatedParams 把 related 的 indexed 数组成员归一为可回放的切片键；
// offset 形式保持原样。
func relatedParams(rawURL *string) (url.Values, bool, error) {
	values, hasNext, err := relatedContinuation.params(rawURL)
	if err != nil || !hasNext {
		return nil, hasNext, err
	}
	if rawOffset := values.Get(pixivapi.ParamOffset); rawOffset != "" {
		value, parseErr := strconv.ParseInt(rawOffset, 10, 64)
		if parseErr != nil || value <= 0 || int64(int(value)) != value {
			return nil, false, pixivapi.MalformedResponse()
		}
		return values, true, nil
	}
	seedIDs, ok := indexedContinuationValues(values, "seed_illust_ids[")
	if !ok {
		return nil, false, pixivapi.MalformedResponse()
	}
	viewed, ok := indexedContinuationValues(values, "viewed[")
	if !ok {
		return nil, false, pixivapi.MalformedResponse()
	}
	return url.Values{
		pixivapi.ParamIllustID:      append([]string(nil), values[pixivapi.ParamIllustID]...),
		pixivapi.ParamSeedIllustIDs: seedIDs,
		pixivapi.ParamViewed:        viewed,
	}, true, nil
}

// indexedContinuationValues 把 viewed[0]=… 这类下标参数按序还原为切片键，
// 要求下标从 0 开始连续且值均为正数。
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

// artworkWire 是 App API 所有 artwork 响应的共同结构。缺少某字段的端点解出 nil，
// 需要区分“未提供”和显式值的字段因此保持指针。
type artworkWire struct {
	ID             int64          `json:"id"`
	Title          string         `json:"title"`
	Caption        string         `json:"caption"`
	Type           string         `json:"type"`
	PageCount      int            `json:"page_count"`
	TotalBookmarks int            `json:"total_bookmarks"`
	TotalView      int            `json:"total_view"`
	XRestrict      int            `json:"x_restrict"`
	User           userWire       `json:"user"`
	Tags           []tagWire      `json:"tags"`
	ImageURLs      imageURLsWire  `json:"image_urls"`
	MetaSinglePage singlePageWire `json:"meta_single_page"`
	MetaPages      []metaPageWire `json:"meta_pages"`
	AIType         int            `json:"-"`
	CreateDate     string         `json:"create_date"`
	Width          int            `json:"width"`
	Height         int            `json:"height"`
	Tools          []string       `json:"tools"`

	IsBookmarked          *bool     `json:"is_bookmarked"`
	IsMuted               *bool     `json:"is_muted"`
	Visible               *bool     `json:"visible"`
	SanityLevel           *int      `json:"sanity_level"`
	RestrictionAttributes *[]string `json:"restriction_attributes"`
	Series                *struct {
		ID    int64  `json:"id"`
		Title string `json:"title"`
	} `json:"series"`
	TotalComments *int `json:"total_comments"`
}

// UnmarshalJSON 同时接受当前的 illust_ai_type 与旧版 ai_type；前者优先。
func (w *artworkWire) UnmarshalJSON(data []byte) error {
	type wire artworkWire
	aux := struct {
		*wire
		IllustAIType *int `json:"illust_ai_type"`
		LegacyAIType *int `json:"ai_type"`
	}{wire: (*wire)(w)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	switch {
	case aux.IllustAIType != nil:
		w.AIType = *aux.IllustAIType
	case aux.LegacyAIType != nil:
		w.AIType = *aux.LegacyAIType
	default:
		w.AIType = 0
	}
	return nil
}

type artworkDetailWire struct {
	Illust *artworkWire `json:"illust"`
}

type artworkListWire struct {
	Illusts pixivapi.RequiredList[artworkWire] `json:"illusts"`
	NextURL *string                            `json:"next_url"`
}

type artworkSeriesWire struct {
	SeriesDetail *struct {
		User userWire `json:"user"`
	} `json:"illust_series_detail"`
	Illusts pixivapi.RequiredList[artworkWire] `json:"illusts"`
	NextURL *string                            `json:"next_url"`
}

type trendingArtworksWire struct {
	Tags pixivapi.RequiredList[struct {
		Tag            string                               `json:"tag"`
		TranslatedName string                               `json:"translated_name"`
		Illust         pixivapi.RequiredObject[artworkWire] `json:"illust"`
	}] `json:"trend_tags"`
}

type ugoiraZipURLsWire struct {
	Medium   string `json:"medium"`
	Original string `json:"original"`
}

type ugoiraFrameWire struct {
	File  string `json:"file"`
	Delay int    `json:"delay"`
}

type ugoiraMetadataWire struct {
	ZipURLs pixivapi.RequiredObject[ugoiraZipURLsWire] `json:"zip_urls"`
	Frames  pixivapi.RequiredList[ugoiraFrameWire]     `json:"frames"`
}

type ugoiraResponseWire struct {
	Metadata pixivapi.RequiredObject[ugoiraMetadataWire] `json:"ugoira_metadata"`
}

// mapArtwork 把一份上游 artwork 响应映射为公开模型，并在受 policy 校验的边界
// 创建资源引用。
func (c *Client) mapArtwork(value artworkWire) (Artwork, error) {
	published, err := parseUTCTime(value.CreateDate)
	if err != nil {
		return Artwork{}, newError("Artwork", sdk.MalformedUpstreamResponse, "invalid publish time")
	}
	kind, raw := artworkKind(value.Type)
	result := Artwork{
		ID:             value.ID,
		Title:          value.Title,
		Caption:        value.Caption,
		Kind:           kind,
		RawKind:        raw,
		Tags:           mapTags(value.Tags),
		User:           c.mapUser(value.User),
		PublishedAt:    published,
		TotalBookmarks: value.TotalBookmarks,
		TotalViews:     value.TotalView,
		Width:          value.Width,
		Height:         value.Height,
		PageCount:      value.PageCount,
		XRestrict:      value.XRestrict,
		AIType:         value.AIType,
		Tools:          append([]string(nil), value.Tools...),

		IsBookmarked:          cloneValue(value.IsBookmarked),
		IsMuted:               cloneValue(value.IsMuted),
		Visible:               cloneValue(value.Visible),
		SanityLevel:           cloneValue(value.SanityLevel),
		RestrictionAttributes: cloneStringSlice(value.RestrictionAttributes),
		TotalComments:         cloneValue(value.TotalComments),
	}
	if value.Series != nil {
		result.Series = &ArtworkSeriesSummary{ID: value.Series.ID, Title: value.Series.Title}
	}
	result.Cover, err = c.artworkCover(value)
	if err != nil {
		return Artwork{}, err
	}
	// Register only variants resolved from this trusted, policy-validated artwork.
	for _, page := range append([]int{-1}, artworkPageIndexes(value)...) {
		for _, variant := range []string{"regular", "small", "thumb", "mini"} {
			raw, e := artworkResourceURL(value, page, variant)
			if e != nil {
				continue
			}
			if _, e = c.newResourceWithVariant("artwork", value.ID, page, variant, raw); e != nil {
				return Artwork{}, e
			}
		}
	}
	return result, nil
}

func (c *Client) artworkCover(value artworkWire) (ImageResource, error) {
	for _, candidate := range []struct {
		variant string
		value   string
	}{
		{variant: "original", value: value.ImageURLs.Original},
		{variant: "large", value: value.ImageURLs.Large},
		{variant: "medium", value: value.ImageURLs.Medium},
		{variant: "square_medium", value: value.ImageURLs.SquareMedium},
	} {
		if candidate.value == "" {
			continue
		}
		resource, err := c.newResourceWithVariant("artwork", value.ID, -1, candidate.variant, candidate.value)
		if err != nil {
			return ImageResource{}, err
		}
		return ImageResource{Resource: resource, Variant: candidate.variant, Width: value.Width, Height: value.Height}, nil
	}
	return ImageResource{}, nil
}

func artworkPageIndexes(value artworkWire) []int {
	indexes := make([]int, 0, len(value.MetaPages)+1)
	for index := range value.MetaPages {
		indexes = append(indexes, index)
	}
	if len(indexes) == 0 && value.MetaSinglePage.OriginalImageURL != "" {
		indexes = append(indexes, 0)
	}
	return indexes
}

// mapArtworkPages 只建立详情响应的页资源；列表响应不解析页。
func (c *Client) mapArtworkPages(value artworkWire) ([]ArtworkPage, error) {
	if len(value.MetaPages) > 0 {
		pages := make([]ArtworkPage, 0, len(value.MetaPages))
		for index, page := range value.MetaPages {
			imageURL := firstArtworkImageURL(page.ImageURLs)
			if imageURL == "" {
				return nil, newError("ArtworkPages", sdk.MalformedUpstreamResponse, "page has no image URL")
			}
			resource, err := c.newResourceWithVariant("artwork", value.ID, index, "original", imageURL)
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
		return nil, nil
	}
	resource, err := c.newResourceWithVariant("artwork", value.ID, 0, "original", value.MetaSinglePage.OriginalImageURL)
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

func firstArtworkImageURL(value imageURLsWire) string {
	for _, candidate := range []string{value.Original, value.Large, value.Medium, value.SquareMedium} {
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

// mapUgoiraMetadata 校验并映射可播放动图的压缩包与帧信息。
func (c *Client) mapUgoiraMetadata(artworkID int64, value ugoiraMetadataWire) (UgoiraMetadata, error) {
	if artworkID <= 0 || !value.ZipURLs.Present || !value.ZipURLs.Valid ||
		(value.ZipURLs.Value.Medium == "" && value.ZipURLs.Value.Original == "") ||
		!value.Frames.Present || !value.Frames.Valid || len(value.Frames.Items) == 0 {
		return UgoiraMetadata{}, pixivapi.MalformedResponse()
	}
	archives := make([]UgoiraArchive, 0, 2)
	seenQuality := map[string]bool{}
	for _, candidate := range []struct {
		quality UgoiraQuality
		url     string
	}{
		{UgoiraQualityMedium, value.ZipURLs.Value.Medium},
		{UgoiraQualityOriginal, value.ZipURLs.Value.Original},
	} {
		if candidate.url == "" || seenQuality[string(candidate.quality)] {
			continue
		}
		resource, err := c.newResourceWithVariant("ugoira_archive", artworkID, -1, string(candidate.quality), candidate.url)
		if err != nil {
			return UgoiraMetadata{}, err
		}
		archives = append(archives, UgoiraArchive{Quality: candidate.quality, Resource: resource})
		seenQuality[string(candidate.quality)] = true
	}
	if len(archives) == 0 {
		return UgoiraMetadata{}, pixivapi.MalformedResponse()
	}
	frames := make([]UgoiraFrame, 0, len(value.Frames.Items))
	seenFile := map[string]bool{}
	for _, frame := range value.Frames.Items {
		if !safeArtworkArchiveFilename(frame.File) || seenFile[frame.File] {
			return UgoiraMetadata{}, pixivapi.MalformedResponse()
		}
		seenFile[frame.File] = true
		frames = append(frames, UgoiraFrame{Filename: frame.File, DelayMilliseconds: frame.Delay})
	}
	return UgoiraMetadata{ArtworkID: artworkID, Archives: archives, Frames: frames}, nil
}

// safeArtworkArchiveFilename 拒绝会进入归档解包或落盘边界的帧名：绝对路径、
// 路径穿越、NUL 与空段。
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

func (c *Client) artworkPage(op string, query url.Values, key string, values []artworkWire, nextValue int64, hasNext bool) (sdk.Page[Artwork], error) {
	items := make([]Artwork, len(values))
	for index, value := range values {
		mapped, err := c.mapArtwork(value)
		if err != nil {
			return sdk.Page[Artwork]{}, err
		}
		items[index] = mapped
	}
	next, err := c.buildCursor(op, query, key, nextValue, hasNext)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	return sdk.Page[Artwork]{Items: items, Next: next}, nil
}

// artworkParamsPage 以多参数 continuation 构建 cursor（recommended）。
func (c *Client) artworkParamsPage(op string, query url.Values, values []artworkWire, nextParams url.Values, hasNext bool) (sdk.Page[Artwork], error) {
	items := make([]Artwork, len(values))
	for index, value := range values {
		mapped, err := c.mapArtwork(value)
		if err != nil {
			return sdk.Page[Artwork]{}, err
		}
		items[index] = mapped
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
	ctx = pixivapi.WithOperation(ctx, "Artwork")
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
	ctx = pixivapi.WithOperation(ctx, "ArtworkPages")
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
	query := url.Values{pixivapi.ParamIllustID: {itoa(artworkID)}}
	if err := c.app.GetJSON(ctx, pixivapi.PathIllustDetail, query, &raw); err != nil {
		return artworkDetailWire{}, classifyAppError(err, operation)
	}
	if raw.Illust == nil || raw.Illust.ID <= 0 {
		return artworkDetailWire{}, newError(operation, sdk.MalformedUpstreamResponse, "")
	}
	return raw, nil
}

// RelatedArtworks lists artworks related to one artwork.
func (c *Client) RelatedArtworks(ctx context.Context, request RelatedArtworksRequest) (sdk.Page[Artwork], error) {
	ctx = pixivapi.WithOperation(ctx, "RelatedArtworks")
	if request.ArtworkID <= 0 {
		return sdk.Page[Artwork]{}, newError("RelatedArtworks", sdk.InvalidArgument, "artwork ID must be positive")
	}
	base := url.Values{pixivapi.ParamIllustID: {itoa(request.ArtworkID)}}
	params, err := c.continuationParams("RelatedArtworks", base, request.Cursor)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	query := base
	if params != nil {
		if params.Get(pixivapi.ParamIllustID) != itoa(request.ArtworkID) {
			return sdk.Page[Artwork]{}, newError("RelatedArtworks", sdk.InvalidCursor, "cursor continuation artwork mismatch")
		}
		query = params
	}

	var raw artworkListWire
	if err := c.app.GetJSON(ctx, pixivapi.PathIllustRelated, query, &raw); err != nil {
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
	ctx = pixivapi.WithOperation(ctx, "ArtworkSeries")
	if request.SeriesID <= 0 {
		return sdk.Page[Artwork]{}, newError("ArtworkSeries", sdk.InvalidArgument, "series ID must be positive")
	}
	base := url.Values{pixivapi.ParamIllustSeriesID: {itoa(request.SeriesID)}}
	query := cloneValues(base)
	key, value, err := c.continuationKeyValue("ArtworkSeries", base, request.Cursor, seriesContinuation)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	if value > 0 {
		query.Set(key, itoa(value))
	}

	var raw artworkSeriesWire
	if err := c.app.GetJSON(ctx, pixivapi.PathIllustSeries, query, &raw); err != nil {
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
	ctx = pixivapi.WithOperation(ctx, "ArtworkRanking")
	if request.Mode == "" {
		request.Mode = RankingModeDay
	}
	if err := validateRankingMode("ArtworkRanking", request.Mode); err != nil {
		return sdk.Page[Artwork]{}, err
	}
	if err := validateDate("ArtworkRanking", "date", request.Date); err != nil {
		return sdk.Page[Artwork]{}, err
	}
	base := url.Values{pixivapi.ParamMode: {string(request.Mode)}}
	if request.Date != "" {
		base.Set(pixivapi.ParamDate, request.Date)
	}
	offset, err := c.continuationPositiveOffset("ArtworkRanking", base, request.Cursor)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	query := cloneValues(base)
	if offset > 0 {
		query.Set(pixivapi.ParamOffset, itoa(int64(offset)))
	}

	var raw artworkListWire
	if err := c.app.GetJSON(ctx, pixivapi.PathIllustRanking, query, &raw); err != nil {
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
	return c.artworkPage("ArtworkRanking", base, pixivapi.ParamOffset, items, int64(next), hasNext)
}

// RecommendedArtworks lists recommended artworks. 上游以多参数 next_url
// 表达续页（offset=0 特例 + bookmark 游标 + viewed 下标数组），cursor 以
// 结构化参数集整体回放，不保存 raw next_url。
func (c *Client) RecommendedArtworks(ctx context.Context, request RecommendedArtworksRequest) (sdk.Page[Artwork], error) {
	ctx = pixivapi.WithOperation(ctx, "RecommendedArtworks")
	base := url.Values{}
	params, err := c.continuationParams("RecommendedArtworks", base, request.Cursor)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}

	var raw artworkListWire
	if err := c.app.GetJSON(ctx, pixivapi.PathIllustRecommended, params, &raw); err != nil {
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
	ctx = pixivapi.WithOperation(ctx, "FollowingArtworks")
	restrict, err := normalizeFollowingRestrict("FollowingArtworks", request.Restrict)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	base := url.Values{pixivapi.ParamRestrict: {string(restrict)}}
	offset, err := c.continuationPositiveOffset("FollowingArtworks", base, request.Cursor)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	query := cloneValues(base)
	if offset > 0 {
		query.Set(pixivapi.ParamOffset, itoa(int64(offset)))
	}

	var raw artworkListWire
	if err := c.app.GetJSON(ctx, pixivapi.PathIllustFollow, query, &raw); err != nil {
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
	return c.artworkPage("FollowingArtworks", base, pixivapi.ParamOffset, items, int64(next), hasNext)
}

// LatestArtworks lists the newest artworks.
func (c *Client) LatestArtworks(ctx context.Context, request LatestArtworksRequest) (sdk.Page[Artwork], error) {
	ctx = pixivapi.WithOperation(ctx, "LatestArtworks")
	contentType, err := normalizeLatestArtworkContentType("LatestArtworks", request.ContentType)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	// Bind the resolved content type into the cursor digest so a continuation
	// produced for one feed (e.g. illust) cannot be replayed against another
	// (e.g. manga), which would resume at an offset for the wrong result set.
	base := url.Values{
		pixivapi.ParamContentType: {contentType},
		pixivapi.ParamFilter:      {"for_android"},
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
	if err := c.app.GetJSON(ctx, pixivapi.PathIllustNew, query, &raw); err != nil {
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
	ctx = pixivapi.WithOperation(ctx, "UserArtworks")
	if request.UserID <= 0 {
		return sdk.Page[Artwork]{}, newError("UserArtworks", sdk.InvalidArgument, "user ID must be positive")
	}
	kind, err := normalizeUserArtworkKind("UserArtworks", request.Kind)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	base := url.Values{pixivapi.ParamUserID: {itoa(request.UserID)}, pixivapi.ParamType: {kind}}
	offset, err := c.continuationPositiveOffset("UserArtworks", base, request.Cursor)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	query := cloneValues(base)
	if offset > 0 {
		query.Set(pixivapi.ParamOffset, itoa(int64(offset)))
	}

	var raw artworkListWire
	if err := c.app.GetJSON(ctx, pixivapi.PathUserIllusts, query, &raw); err != nil {
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
	return c.artworkPage("UserArtworks", base, pixivapi.ParamOffset, items, int64(next), hasNext)
}

// MyPixivArtworks lists artworks from the current user's MyPixiv feed.
func (c *Client) MyPixivArtworks(ctx context.Context, request MyPixivArtworksRequest) (sdk.Page[Artwork], error) {
	ctx = pixivapi.WithOperation(ctx, "MyPixivArtworks")
	base := url.Values{}
	offset, err := c.continuationPositiveOffset("MyPixivArtworks", base, request.Cursor)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	query := cloneValues(base)
	if offset > 0 {
		query.Set(pixivapi.ParamOffset, itoa(int64(offset)))
	}

	var raw artworkListWire
	if err := c.app.GetJSON(ctx, pixivapi.PathIllustMyPixiv, query, &raw); err != nil {
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
	return c.artworkPage("MyPixivArtworks", base, pixivapi.ParamOffset, items, int64(next), hasNext)
}

// TrendingArtworkTags lists currently trending artwork tags.
func (c *Client) TrendingArtworkTags(ctx context.Context, request TrendingArtworkTagsRequest) ([]TrendingTag, error) {
	ctx = pixivapi.WithOperation(ctx, "TrendingArtworkTags")
	var raw trendingArtworksWire
	if err := c.app.GetJSON(ctx, pixivapi.PathTrendingTagsIllust, nil, &raw); err != nil {
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
	ctx = pixivapi.WithOperation(ctx, "UgoiraMetadata")
	if request.ArtworkID <= 0 {
		return UgoiraMetadata{}, newError("UgoiraMetadata", sdk.InvalidArgument, "artwork ID must be positive")
	}
	var raw ugoiraResponseWire
	query := url.Values{pixivapi.ParamIllustID: {itoa(request.ArtworkID)}}
	if err := c.app.GetJSON(ctx, pixivapi.PathUgoiraMetadata, query, &raw); err != nil {
		return UgoiraMetadata{}, classifyAppError(err, "UgoiraMetadata")
	}
	if !raw.Metadata.Present || !raw.Metadata.Valid {
		return UgoiraMetadata{}, newError("UgoiraMetadata", sdk.MalformedUpstreamResponse, "")
	}
	metadata, err := c.mapUgoiraMetadata(request.ArtworkID, raw.Metadata.Value)
	if err != nil {
		// 映射阶段返回的已是公开 SDK 错误（例如 ResourceForbidden），不能再按
		// 上游失败重新分类，否则会丢掉原始 reason。
		var sdkErr *sdk.Error
		if errors.As(err, &sdkErr) {
			return UgoiraMetadata{}, err
		}
		return UgoiraMetadata{}, classifyAppError(err, "UgoiraMetadata")
	}
	return metadata, nil
}

// artworkItems 校验 artwork 列表的必需字段并原样返回 wire 项。
func artworkItems(operation string, list pixivapi.RequiredList[artworkWire], requireUser bool) ([]artworkWire, error) {
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

type singlePageWire struct {
	OriginalImageURL string `json:"original_image_url"`
}

type metaPageWire struct {
	Width     int           `json:"width"`
	Height    int           `json:"height"`
	Extension string        `json:"extension"`
	ImageURLs imageURLsWire `json:"image_urls"`
}

func firstAvailable(urls imageURLsWire) string {
	for _, value := range []string{urls.Original, urls.Large, urls.Medium, urls.SquareMedium} {
		if value != "" {
			return value
		}
	}
	return ""
}

func cloneStringSlice(value *[]string) *[]string {
	if value == nil {
		return nil
	}
	cloned := make([]string, len(*value))
	copy(cloned, *value)
	return &cloned
}
