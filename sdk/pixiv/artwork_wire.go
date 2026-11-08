package pixiv

import (
	"encoding/json"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
)

type singlePageWire struct {
	OriginalImageURL string `json:"original_image_url"`
}

type metaPageWire struct {
	Width     int           `json:"width"`
	Height    int           `json:"height"`
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
