package pixiv

import (
	"encoding/json"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
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

// artworkWire is the shape every App API artwork response shares. Endpoints that
// omit a field decode it as nil, so fields whose absence differs from an
// explicit value stay pointers.
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

// UnmarshalJSON accepts both the current illust_ai_type and the earlier ai_type,
// preferring the current one.
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
	Illusts api.RequiredList[artworkWire] `json:"illusts"`
	NextURL *string                       `json:"next_url"`
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
	ZipURLs api.RequiredObject[ugoiraZipURLsWire] `json:"zip_urls"`
	Frames  api.RequiredList[ugoiraFrameWire]     `json:"frames"`
}

type ugoiraResponseWire struct {
	Metadata api.RequiredObject[ugoiraMetadataWire] `json:"ugoira_metadata"`
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

// mapArtwork maps one upstream artwork response to the public model and creates
// resource references through the policy-checked boundary.
func (c *Client) mapArtwork(value artworkWire) (Artwork, error) {
	published, err := parseUTCTime(value.CreateDate)
	if err != nil {
		return Artwork{}, newError("Artwork", sdk.MalformedUpstreamResponse, "invalid publish time")
	}
	user, err := c.mapUser(value.User)
	if err != nil {
		return Artwork{}, err
	}
	kind, raw := artworkKind(value.Type)
	result := Artwork{
		ID:             value.ID,
		Title:          value.Title,
		Caption:        value.Caption,
		Kind:           kind,
		RawKind:        raw,
		Tags:           mapTags(value.Tags),
		User:           user,
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
