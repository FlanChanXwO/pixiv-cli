package pixiv

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// ContentType preserves the upstream artwork category.
type ContentType string

const (
	// ContentTypeIllust selects illustrations.
	ContentTypeIllust ContentType = "illust"
	// ContentTypeManga selects manga.
	ContentTypeManga ContentType = "manga"
	// ContentTypeUgoira selects animated artworks.
	ContentTypeUgoira ContentType = "ugoira"
)

// Tag names an artwork or novel tag, with its optional translation.
type Tag struct {
	Name           string `json:"name"`
	TranslatedName string `json:"translated_name,omitempty"`
}

// ArtworkRef addresses an artwork on the client that created it.
// Its constructor performs no I/O; operations validate the ID and binding.
type ArtworkRef struct {
	ID     int64 `json:"id"`
	client *Client
}

// Artwork returns a reference without performing I/O.
func (c *Client) Artwork(id int64) *ArtworkRef { return &ArtworkRef{ID: id, client: c} }

func (r *ArtworkRef) bound(operation string) error {
	if r == nil || r.client == nil {
		return newError(operation, sdk.InvalidArgument, "artwork is unbound; create a reference through Client.Artwork", nil)
	}
	if r.ID <= 0 {
		return newError(operation, sdk.InvalidArgument, "artwork ID must be positive", nil)
	}
	return nil
}

// UnmarshalJSON clears any previous client binding, including when decoding into a live reference.
func (r *ArtworkRef) UnmarshalJSON(data []byte) error {
	var value struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*r = ArtworkRef{ID: value.ID}
	return nil
}

// Get loads the artwork and all its image pages.
func (r *ArtworkRef) Get(ctx context.Context) (*Artwork, error) {
	if err := r.bound("Artwork.Get"); err != nil {
		return nil, err
	}
	var raw struct {
		Illust *artworkWire `json:"illust"`
	}
	if err := r.client.getJSON(ctx, api.PathIllustDetail, url.Values{api.ParamIllustID: {strconv.FormatInt(r.ID, 10)}}, &raw); err != nil {
		return nil, err
	}
	if raw.Illust == nil || raw.Illust.ID <= 0 {
		return nil, newError("Artwork.Get", sdk.MalformedUpstreamResponse, "missing artwork", nil)
	}
	if raw.Illust.ID != r.ID {
		return nil, newError("Artwork.Get", sdk.MalformedUpstreamResponse, "artwork ID does not match request", nil)
	}
	return r.client.mapArtwork(*raw.Illust)
}

// ArtworkSeriesSummary links an artwork to its bound series and supplied title.
type ArtworkSeriesSummary struct {
	*ArtworkSeriesRef
	Title string `json:"title"`
}

// Artwork carries the same data in lists and details and embeds its account-bound operations.
// Pages link back to this object; Author carries the same client binding.
type Artwork struct {
	*ArtworkRef
	artworkData
}

type artworkData struct {
	Title                 string                `json:"title"`
	Caption               string                `json:"caption"`
	ContentType           ContentType           `json:"content_type"`
	Author                *User                 `json:"author"`
	Tags                  []Tag                 `json:"tags"`
	PublishedAt           time.Time             `json:"published_at"`
	TotalBookmarks        int                   `json:"total_bookmarks"`
	TotalViews            int                   `json:"total_views"`
	Width                 int                   `json:"width"`
	Height                int                   `json:"height"`
	PageCount             int                   `json:"page_count"`
	XRestrict             int                   `json:"x_restrict"`
	AIType                int                   `json:"ai_type"`
	Tools                 []string              `json:"tools"`
	Cover                 Image                 `json:"cover"`
	Pages                 []ArtworkPage         `json:"pages"`
	IsBookmarked          *bool                 `json:"is_bookmarked,omitempty"`
	IsMuted               *bool                 `json:"is_muted,omitempty"`
	Visible               *bool                 `json:"visible,omitempty"`
	SanityLevel           *int                  `json:"sanity_level,omitempty"`
	RestrictionAttributes *[]string             `json:"restriction_attributes,omitempty"`
	Series                *ArtworkSeriesSummary `json:"series,omitempty"`
	TotalComments         *int                  `json:"total_comments,omitempty"`
}

// UnmarshalJSON decodes data without retaining an earlier client binding.
func (a *Artwork) UnmarshalJSON(data []byte) error {
	var value struct {
		artworkData
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*a = Artwork{ArtworkRef: &ArtworkRef{ID: value.ID}, artworkData: value.artworkData}
	for i := range a.Pages {
		a.Pages[i].Artwork = a
	}
	return nil
}

// ArtworkPage identifies a zero-based image page and its owning artwork.
// Ugoira artworks expose exactly one page; animation metadata is loaded separately.
type ArtworkPage struct {
	Artwork   *Artwork `json:"-"`
	PageIndex int      `json:"page_index"`
	Original  Image    `json:"original"`
	Width     int      `json:"width"`
	Height    int      `json:"height"`
}

// UnmarshalJSON clears the previous owner; decoding a complete Artwork restores
// the page's link to that decoded, unbound artwork.
func (p *ArtworkPage) UnmarshalJSON(data []byte) error {
	type value ArtworkPage
	var decoded value
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*p = ArtworkPage(decoded)
	return nil
}
