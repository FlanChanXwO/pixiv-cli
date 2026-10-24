// Pixiv App API 的 artwork 与 novel wire 结构、响应字段映射，以及每个端点的
// next_url continuation Spec 与解析函数。

package pixiv

import (
	"encoding/json"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixivapi"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork"
	endpointcontinuation "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/continuation"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/novel"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

type profileImageURLsWire struct {
	Medium *string `json:"medium"`
}

type userWire struct {
	ID               int64                `json:"id"`
	Name             string               `json:"name"`
	Account          string               `json:"account"`
	Comment          string               `json:"comment"`
	IsFollowed       bool                 `json:"is_followed"`
	ProfileImageURLs profileImageURLsWire `json:"profile_image_urls"`
}

type tagWire struct {
	Name           string `json:"name"`
	TranslatedName string `json:"translated_name"`
}

type imageURLsWire struct {
	SquareMedium string `json:"square_medium"`
	Medium       string `json:"medium"`
	Large        string `json:"large"`
	Original     string `json:"original"`
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

type commentWire struct {
	ID            int64        `json:"id"`
	User          userWire     `json:"user"`
	Comment       string       `json:"comment"`
	Caption       string       `json:"caption"`
	Date          string       `json:"date"`
	CreateDate    string       `json:"created_at"`
	ParentComment *commentWire `json:"parent_comment"`
}

type commentAccessControlWire struct {
	CanComment bool `json:"can_comment"`
	IsLocked   bool `json:"is_locked"`
}

// commentsWire 是 illust 与 novel 评论列表的共同响应结构。
type commentsWire struct {
	Comments      pixivapi.RequiredList[commentWire] `json:"comments"`
	NextURL       *string                            `json:"next_url"`
	TotalComments *int64                             `json:"total_comments"`
	AccessControl *commentAccessControlWire          `json:"access_control"`
	// CommentAccessControl 是当前 App API 标量；与 legacy object 分开解码，
	// 避免猜测其业务含义。
	CommentAccessControl *int64 `json:"comment_access_control"`
}

// novelWire 是 App API 所有 novel 响应的共同结构。search 要求
// x_restrict/text_length/is_original 存在，其余端点把缺失值当作零值；因此这三
// 个字段保持指针，由调用端点决定是否要求。
type novelWire struct {
	ID             int64         `json:"id"`
	Title          string        `json:"title"`
	Caption        string        `json:"caption"`
	XRestrict      *int          `json:"x_restrict"`
	TextLength     *int          `json:"text_length"`
	IsOriginal     *bool         `json:"is_original"`
	User           userWire      `json:"user"`
	Tags           []tagWire     `json:"tags"`
	ImageURLs      imageURLsWire `json:"image_urls"`
	CreateDate     string        `json:"create_date"`
	TotalBookmarks int           `json:"total_bookmarks"`
	TotalView      int           `json:"total_view"`
}

type novelSeriesRefWire struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

type novelDetailWire struct {
	Novel      *novelWire          `json:"novel"`
	SeriesNext *novelSeriesRefWire `json:"series_next"`
	SeriesPrev *novelSeriesRefWire `json:"series_prev"`
}

type novelListWire struct {
	Novels  pixivapi.RequiredList[novelWire] `json:"novels"`
	NextURL *string                          `json:"next_url"`
}

type novelSeriesDetailWire struct {
	ID          int64    `json:"id"`
	Title       string   `json:"title"`
	Caption     string   `json:"caption"`
	User        userWire `json:"user"`
	IsConcluded bool     `json:"is_concluded"`
}

type novelSeriesWire struct {
	Detail  *novelSeriesDetailWire           `json:"novel_series_detail"`
	Novels  pixivapi.RequiredList[novelWire] `json:"novels"`
	NextURL *string                          `json:"next_url"`
}

type bookmarkTagWire struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type bookmarkTagsWire struct {
	Tags    pixivapi.RequiredList[bookmarkTagWire] `json:"bookmark_tags"`
	NextURL *string                                `json:"next_url"`
}

type bookmarkDetailWire struct {
	Detail *struct {
		IsBookmarked *bool  `json:"is_bookmarked"`
		Restrict     string `json:"restrict"`
		Tags         []struct {
			Name         string `json:"name"`
			IsRegistered bool   `json:"is_registered"`
		} `json:"tags"`
	} `json:"bookmark_detail"`
}

type commentMutationWire struct {
	CommentID *int64 `json:"comment_id"`
	Comment   *struct {
		ID *int64 `json:"id"`
	} `json:"comment"`
}

func (w commentMutationWire) commentID() *int64 {
	if w.CommentID != nil {
		return w.CommentID
	}
	if w.Comment != nil {
		return w.Comment.ID
	}
	return nil
}

// mapUser 是所有 artwork 与 artwork comment 响应的唯一 user 映射。
func (c *Client) mapUser(value userWire) User {
	result := User{
		ID:         value.ID,
		Name:       value.Name,
		Account:    value.Account,
		Comment:    value.Comment,
		IsFollowed: value.IsFollowed,
	}
	if value.ProfileImageURLs.Medium != nil && *value.ProfileImageURLs.Medium != "" {
		if resource, err := c.newResourceWithVariant("user_profile", value.ID, -1, "medium", *value.ProfileImageURLs.Medium); err == nil {
			result.ProfileImage = ImageResource{Resource: resource, Variant: "medium"}
		}
	}
	return result
}

func mapTags(values []tagWire) []Tag {
	result := make([]Tag, len(values))
	for index, value := range values {
		result[index] = Tag{Name: value.Name, TranslatedName: value.TranslatedName}
	}
	return result
}

// mapNovel 把一份上游 novel 响应映射为公开模型。cover 与 user profile 都经
// 受 policy 校验的资源边界创建。
func (c *Client) mapNovel(value novelWire) (Novel, error) {
	published, err := parseUTCTime(value.CreateDate)
	if err != nil {
		return Novel{}, newError("Novel", sdk.MalformedUpstreamResponse, "invalid publish time")
	}
	result := Novel{
		ID:             value.ID,
		Title:          value.Title,
		Caption:        value.Caption,
		User:           c.mapUser(value.User),
		Tags:           mapTags(value.Tags),
		PublishedAt:    published,
		XRestrict:      intValue(value.XRestrict),
		TextLength:     intValue(value.TextLength),
		IsOriginal:     value.IsOriginal != nil && *value.IsOriginal,
		TotalBookmarks: value.TotalBookmarks,
		TotalViews:     value.TotalView,
	}
	for _, size := range []struct {
		variant string
		value   string
	}{
		{"original", value.ImageURLs.Original},
		{"large", value.ImageURLs.Large},
		{"medium", value.ImageURLs.Medium},
		{"square_medium", value.ImageURLs.SquareMedium},
	} {
		if size.value == "" {
			continue
		}
		resource, err := c.newResourceWithVariant("novel_cover", value.ID, -1, size.variant, size.value)
		if err != nil {
			return Novel{}, err
		}
		result.Cover = ImageResource{Resource: resource, Variant: size.variant}
		break
	}
	return result, nil
}

func (c *Client) novelItems(operation string, list pixivapi.RequiredList[novelWire], requireSearchFields bool) ([]Novel, error) {
	if !list.Present || !list.Valid {
		return nil, newError(operation, sdk.MalformedUpstreamResponse, "")
	}
	items := make([]Novel, len(list.Items))
	for index, value := range list.Items {
		if value.ID <= 0 || value.User.ID <= 0 ||
			requireSearchFields && (value.XRestrict == nil || value.TextLength == nil || value.IsOriginal == nil) {
			return nil, newError(operation, sdk.MalformedUpstreamResponse, "")
		}
		mapped, err := c.mapNovel(value)
		if err != nil {
			return nil, err
		}
		items[index] = mapped
	}
	return items, nil
}

// novelWireFromEntity 是 internal novel 实体到 wire 的临时桥接：user 端点
// 仍在读取 novel.UserSummary，G08 删除 internal novel 实体后一并删除。
func novelWireFromEntity(value novel.Novel) novelWire {
	tags := make([]tagWire, len(value.Tags))
	for index, tag := range value.Tags {
		tags[index] = tagWire{Name: tag.Name, TranslatedName: tag.TranslatedName}
	}
	xRestrict, textLength, isOriginal := value.XRestrict, value.TextLength, value.IsOriginal
	return novelWire{
		ID: value.ID, Title: value.Title, Caption: value.Caption,
		XRestrict: &xRestrict, TextLength: &textLength, IsOriginal: &isOriginal,
		User: userWire{
			ID: value.User.ID, Name: value.User.Name, Account: value.User.Account,
			Comment: value.User.Comment, IsFollowed: value.User.IsFollowed,
			ProfileImageURLs: profileImageURLsWire{Medium: cloneValue(value.User.ProfileImageURLs.Medium)},
		},
		Tags: tags, ImageURLs: imageURLsWire(value.ImageURLs),
		CreateDate: value.CreateDate, TotalBookmarks: value.TotalBookmarks, TotalView: value.TotalView,
	}
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

// mapComment 映射一条 artwork comment，并解析其父评论链。
func (c *Client) mapComment(value commentWire) (Comment, error) {
	created, err := parseUTCTime(commentDate(value))
	if err != nil {
		return Comment{}, newError("Comment", sdk.MalformedUpstreamResponse, "invalid comment time")
	}
	result := Comment{ID: value.ID, User: c.mapUser(value.User), Comment: value.Comment, CreatedAt: created}
	if result.Comment == "" {
		result.Comment = value.Caption
	}
	if value.ParentComment != nil {
		parent, err := c.mapComment(*value.ParentComment)
		if err != nil {
			return Comment{}, err
		}
		result.ParentComment = &parent
	}
	return result, nil
}

func commentDate(value commentWire) string {
	if value.Date != "" {
		return value.Date
	}
	return value.CreateDate
}

func validCommentChain(value commentWire) bool {
	for {
		if value.ID <= 0 {
			return false
		}
		if value.ParentComment == nil {
			return true
		}
		value = *value.ParentComment
	}
}

// artworkWireFromEntity 是 internal endpoint 实体到 wire 的临时桥接：user 与
// novel 端点尚未并入 SDK，仍需读取 artwork.Artwork。G08 删除 internal artwork
// 实体后连同本函数一起删除。
func artworkWireFromEntity(value artwork.Artwork) artworkWire {
	tags := make([]tagWire, len(value.Tags))
	for index, tag := range value.Tags {
		tags[index] = tagWire{Name: tag.Name, TranslatedName: tag.TranslatedName}
	}
	pages := make([]metaPageWire, len(value.MetaPages))
	for index, page := range value.MetaPages {
		pages[index] = metaPageWire{
			Width: page.Width, Height: page.Height, Extension: page.Extension,
			ImageURLs: imageURLsWire(page.ImageURLs),
		}
	}
	var series *struct {
		ID    int64  `json:"id"`
		Title string `json:"title"`
	}
	if value.Series != nil {
		series = &struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		}{ID: value.Series.ID, Title: value.Series.Title}
	}
	return artworkWire{
		ID: value.ID, Title: value.Title, Caption: value.Caption, Type: value.Type,
		PageCount: value.PageCount, TotalBookmarks: value.TotalBookmarks, TotalView: value.TotalView,
		XRestrict: value.XRestrict,
		User: userWire{
			ID: value.User.ID, Name: value.User.Name, Account: value.User.Account,
			Comment: value.User.Comment, IsFollowed: value.User.IsFollowed,
			ProfileImageURLs: profileImageURLsWire{Medium: cloneValue(value.User.ProfileImageURLs.Medium)},
		},
		Tags: tags, ImageURLs: imageURLsWire(value.ImageURLs),
		MetaSinglePage: singlePageWire{OriginalImageURL: value.MetaSinglePage.OriginalImageURL},
		MetaPages:      pages, AIType: value.AIType, CreateDate: value.CreateDate,
		Width: value.Width, Height: value.Height, Tools: append([]string(nil), value.Tools...),
		IsBookmarked: value.IsBookmarked, IsMuted: value.IsMuted, Visible: value.Visible,
		SanityLevel: cloneValue(value.SanityLevel), RestrictionAttributes: value.RestrictionAttributes,
		Series: series, TotalComments: value.TotalComments,
	}
}

// continuationSpec 描述单条 artwork endpoint 的 next_url 白名单。
type continuationSpec struct {
	path      string
	keys      []string
	allowZero bool
	query     []string
	prefixes  []string
	ignored   []string
}

var (
	relatedContinuation = continuationSpec{
		path:     pixivapi.PathIllustRelated,
		keys:     []string{pixivapi.ParamOffset},
		query:    []string{pixivapi.ParamIllustID},
		prefixes: []string{"seed_illust_ids[", "viewed["},
	}
	recommendedContinuation = continuationSpec{
		path: pixivapi.PathIllustRecommended,
		keys: []string{
			pixivapi.ParamOffset, pixivapi.ParamMinRecentIllusts, pixivapi.ParamMaxRecommend,
			pixivapi.ParamIncludeRanking, pixivapi.ParamIncludePrivacy,
		},
		allowZero: true,
		ignored:   []string{"viewed["},
	}
	rankingContinuation = continuationSpec{
		path:  pixivapi.PathIllustRanking,
		keys:  []string{pixivapi.ParamOffset},
		query: []string{pixivapi.ParamMode, pixivapi.ParamDate},
	}
	commentsContinuation = continuationSpec{
		path:  pixivapi.PathIllustComments,
		keys:  []string{pixivapi.ParamOffset},
		query: []string{pixivapi.ParamIllustID},
	}
	bookmarkArtworksContinuation = continuationSpec{
		path:  pixivapi.PathUserBookmarksIllust,
		keys:  []string{pixivapi.ParamMaxBookmarkID},
		query: []string{pixivapi.ParamUserID, pixivapi.ParamRestrict, pixivapi.ParamTag},
	}
	bookmarkTagsContinuation = continuationSpec{
		path:  pixivapi.PathUserBookmarkTags,
		keys:  []string{pixivapi.ParamOffset},
		query: []string{pixivapi.ParamUserID, pixivapi.ParamRestrict},
	}
	seriesContinuation = continuationSpec{
		path:  pixivapi.PathIllustSeries,
		keys:  []string{pixivapi.ParamOffset, pixivapi.ParamLastOrder},
		query: []string{pixivapi.ParamIllustSeriesID},
	}
	followingContinuation = continuationSpec{
		path:  pixivapi.PathIllustFollow,
		keys:  []string{pixivapi.ParamOffset},
		query: []string{pixivapi.ParamRestrict},
	}
	latestContinuation = continuationSpec{
		path:  pixivapi.PathIllustNew,
		keys:  []string{pixivapi.ParamMaxIllustID, pixivapi.ParamOffset},
		query: []string{pixivapi.ParamContentType, pixivapi.ParamFilter},
	}
	userArtworksContinuation = continuationSpec{
		path:  pixivapi.PathUserIllusts,
		keys:  []string{pixivapi.ParamOffset},
		query: []string{pixivapi.ParamUserID, pixivapi.ParamType},
	}
	myPixivContinuation = continuationSpec{
		path: pixivapi.PathIllustMyPixiv,
		keys: []string{pixivapi.ParamOffset},
	}
	novelSearchContinuation = continuationSpec{
		path: pixivapi.PathSearchNovel,
		keys: []string{pixivapi.ParamOffset},
		query: []string{
			pixivapi.ParamWord, pixivapi.ParamSearchTarget, pixivapi.ParamSort, pixivapi.ParamDuration,
		},
	}
	novelRankingContinuation = continuationSpec{
		path:  pixivapi.PathNovelRanking,
		keys:  []string{pixivapi.ParamOffset},
		query: []string{pixivapi.ParamFilter, pixivapi.ParamMode},
	}
	novelRecommendedContinuation = continuationSpec{
		path: pixivapi.PathNovelRecommended,
		keys: []string{
			pixivapi.ParamOffset, pixivapi.ParamAlreadyRecommended,
			pixivapi.ParamMaxRecommend, pixivapi.ParamIncludeRankingNovels, pixivapi.ParamIncludePrivacy,
		},
		allowZero: true,
	}
	novelSeriesContinuation = continuationSpec{
		path:  pixivapi.PathNovelSeries,
		keys:  []string{pixivapi.ParamLastOrder},
		query: []string{pixivapi.ParamSeriesID},
	}
	novelCommentsContinuation = continuationSpec{
		path:  pixivapi.PathNovelComments,
		keys:  []string{pixivapi.ParamOffset},
		query: []string{pixivapi.ParamNovelID},
	}
	novelFollowingContinuation = continuationSpec{
		path:  pixivapi.PathNovelFollow,
		keys:  []string{pixivapi.ParamOffset},
		query: []string{pixivapi.ParamRestrict},
	}
	novelLatestContinuation = continuationSpec{
		path:  pixivapi.PathNovelNew,
		keys:  []string{pixivapi.ParamMaxNovelID},
		query: []string{pixivapi.ParamFilter},
	}
	novelMyPixivContinuation = continuationSpec{
		path: pixivapi.PathNovelMyPixiv,
		keys: []string{pixivapi.ParamOffset},
	}
	searchContinuation = continuationSpec{
		path: pixivapi.PathSearchIllust,
		keys: []string{pixivapi.ParamOffset},
		query: []string{
			pixivapi.ParamWord, pixivapi.ParamSearchTarget, pixivapi.ParamSort,
			pixivapi.ParamDuration, pixivapi.ParamStartDate, pixivapi.ParamEndDate,
			pixivapi.ParamSearchAIType, pixivapi.ParamRatioPattern, pixivapi.ParamContentType,
			pixivapi.ParamTool, pixivapi.ParamBookmarkNumMin, pixivapi.ParamBookmarkNumMax,
			pixivapi.ParamWidthMin, pixivapi.ParamWidthMax, pixivapi.ParamHeightMin, pixivapi.ParamHeightMax,
		},
	}
)

func (s continuationSpec) spec() endpointcontinuation.Spec {
	return endpointcontinuation.Spec{
		Path:               s.path,
		Keys:               s.keys,
		AllowZero:          s.allowZero,
		AllowedQueryKeys:   s.query,
		AllowedKeyPrefixes: s.prefixes,
		IgnoredKeyPrefixes: s.ignored,
	}
}

// offset 解析只允许单一 offset 的续页；offset 必须是正 int。
func (s continuationSpec) offset(rawURL *string) (int, bool, error) {
	value, hasNext, err := s.value(rawURL)
	if err != nil || !hasNext {
		return 0, hasNext, err
	}
	if value <= 0 || int64(int(value)) != value {
		return 0, false, pixivapi.MalformedResponse()
	}
	return int(value), true, nil
}

// keyValue 返回单一 continuation 的 key 与值，供 series 与 latest 这类允许
// 两种续页形式的端点构建 cursor。
func (s continuationSpec) keyValue(rawURL *string) (string, int64, bool, error) {
	if rawURL == nil {
		return "", 0, false, nil
	}
	key, value, err := endpointcontinuation.Parse(*rawURL, s.spec())
	if err != nil || value <= 0 {
		return "", 0, false, pixivapi.MalformedResponse()
	}
	if key == pixivapi.ParamOffset && int64(int(value)) != value {
		return "", 0, false, pixivapi.MalformedResponse()
	}
	return key, value, true, nil
}

// value 解析单一 typed continuation key，并拒绝缺失与重复。
func (s continuationSpec) value(rawURL *string) (int64, bool, error) {
	if rawURL == nil {
		return 0, false, nil
	}
	_, value, err := endpointcontinuation.Parse(*rawURL, s.spec())
	if err != nil || value <= 0 {
		return 0, false, pixivapi.MalformedResponse()
	}
	if s.keys[0] == pixivapi.ParamOffset && int64(int(value)) != value {
		return 0, false, pixivapi.MalformedResponse()
	}
	return value, true, nil
}

// params 整体回放多参数续页，返回上游 next_url 的完整查询集。
func (s continuationSpec) params(rawURL *string) (url.Values, bool, error) {
	if rawURL == nil {
		return nil, false, nil
	}
	values, hasNext, err := endpointcontinuation.ParseParams(*rawURL, s.spec())
	if err != nil || !hasNext {
		return nil, false, pixivapi.MalformedResponse()
	}
	return values, true, nil
}

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

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}
