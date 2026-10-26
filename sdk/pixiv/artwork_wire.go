// Artwork and ugoira wire shapes shared by endpoint, preview, search and resource readers.
// Shared artwork mapping lives here; endpoint-specific declarations stay in artwork.go.

package pixiv

import (
	"encoding/json"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixivapi"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

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

type artworkListWire struct {
	Illusts pixivapi.RequiredList[artworkWire] `json:"illusts"`
	NextURL *string                            `json:"next_url"`
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
			if _, e = c.newResource("artwork", value.ID, page, variant, raw); e != nil {
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
		resource, err := c.newResource("artwork", value.ID, -1, candidate.variant, candidate.value)
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
