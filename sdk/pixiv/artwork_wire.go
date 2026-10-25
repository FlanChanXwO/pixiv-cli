// Artwork wire responses and model mapping.
// Endpoint methods live in artwork.go; shared mappings in response.go.

package pixiv

import (
	"encoding/json"
	"path"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixivapi"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

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

func (c *Client) mapArtworks(values []artworkWire) ([]Artwork, error) {
	items := make([]Artwork, len(values))
	for index, value := range values {
		mapped, err := c.mapArtwork(value)
		if err != nil {
			return nil, err
		}
		items[index] = mapped
	}
	return items, nil
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
		Tools:          value.Tools,
		Pages:          []ArtworkPage{},

		IsBookmarked:          value.IsBookmarked,
		IsMuted:               value.IsMuted,
		Visible:               value.Visible,
		SanityLevel:           value.SanityLevel,
		RestrictionAttributes: value.RestrictionAttributes,
		TotalComments:         value.TotalComments,
	}
	if result.Tools == nil {
		result.Tools = []string{}
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
			imageURL := firstImageURL(page.ImageURLs)
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
		return []ArtworkPage{}, nil
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

func firstImageURL(value imageURLsWire) string {
	for _, candidate := range []string{value.Original, value.Large, value.Medium, value.SquareMedium} {
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

// mapUgoiraMetadata 校验并映射可播放动图的压缩包与帧信息。
func (c *Client) mapUgoiraMetadata(artworkID int64, value ugoiraMetadataWire) (UgoiraMetadata, error) {
	if !value.ZipURLs.Present || !value.ZipURLs.Valid ||
		(value.ZipURLs.Value.Medium == "" && value.ZipURLs.Value.Original == "") ||
		!value.Frames.Present || !value.Frames.Valid || len(value.Frames.Items) == 0 {
		return UgoiraMetadata{}, pixivapi.MalformedResponse()
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
		resource, err := c.newResourceWithVariant("ugoira_archive", artworkID, -1, string(candidate.quality), candidate.url)
		if err != nil {
			return UgoiraMetadata{}, err
		}
		archives = append(archives, UgoiraArchive{Quality: candidate.quality, Resource: resource})
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
