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

// NovelRef addresses a novel on the client that created it.
type NovelRef struct {
	ID     int64 `json:"id"`
	client *Client
}

// Novel returns a reference without network or credential I/O.
func (c *Client) Novel(id int64) *NovelRef { return &NovelRef{ID: id, client: c} }

func (r *NovelRef) bound(operation string) error {
	if r == nil || r.client == nil {
		return newError(operation, sdk.InvalidArgument, "novel is unbound; create a reference through Client.Novel", nil)
	}
	if r.ID <= 0 {
		return newError(operation, sdk.InvalidArgument, "novel ID must be positive", nil)
	}
	return nil
}

// UnmarshalJSON restores the identity and clears any earlier account binding.
func (r *NovelRef) UnmarshalJSON(data []byte) error {
	var value struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*r = NovelRef{ID: value.ID}
	return nil
}

// NovelSummary links to an adjacent novel with the title supplied by the series.
type NovelSummary struct {
	*NovelRef
	Title string `json:"title"`
}

// UnmarshalJSON restores a summary without retaining its earlier client.
func (n *NovelSummary) UnmarshalJSON(data []byte) error {
	var value struct {
		ID    int64  `json:"id"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*n = NovelSummary{NovelRef: &NovelRef{ID: value.ID}, Title: value.Title}
	return nil
}

// Novel carries the same metadata in details and lists, with account-bound operations.
type Novel struct {
	*NovelRef
	novelData
}

type novelData struct {
	Title          string        `json:"title"`
	Caption        string        `json:"caption"`
	Author         *User         `json:"author"`
	Tags           []Tag         `json:"tags"`
	Cover          Image         `json:"cover"`
	PublishedAt    time.Time     `json:"published_at"`
	XRestrict      int           `json:"x_restrict"`
	TextLength     int           `json:"text_length"`
	IsOriginal     bool          `json:"is_original"`
	TotalBookmarks int           `json:"total_bookmarks"`
	TotalViews     int           `json:"total_views"`
	SeriesNext     *NovelSummary `json:"series_next,omitempty"`
	SeriesPrevious *NovelSummary `json:"series_prev,omitempty"`
}

// UnmarshalJSON restores metadata while clearing the object and associated resource bindings.
func (n *Novel) UnmarshalJSON(data []byte) error {
	var value struct {
		novelData
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*n = Novel{NovelRef: &NovelRef{ID: value.ID}, novelData: value.novelData}
	return nil
}

// Get loads the novel's metadata and links to adjacent series entries.
func (r *NovelRef) Get(ctx context.Context) (*Novel, error) {
	if err := r.bound("Novel.Get"); err != nil {
		return nil, err
	}
	var raw struct {
		Novel *novelWire `json:"novel"`
		Next  *struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		} `json:"series_next"`
		Previous *struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		} `json:"series_prev"`
	}
	if err := r.client.getJSON(ctx, api.PathNovelDetail, url.Values{api.ParamNovelID: {strconv.FormatInt(r.ID, 10)}}, &raw); err != nil {
		return nil, err
	}
	if raw.Novel == nil || raw.Novel.ID != r.ID || raw.Next != nil && raw.Next.ID <= 0 || raw.Previous != nil && raw.Previous.ID <= 0 {
		return nil, newError("Novel.Get", sdk.MalformedUpstreamResponse, "invalid novel detail", nil)
	}
	novel, err := r.client.mapNovel(*raw.Novel)
	if err != nil {
		return nil, err
	}
	if raw.Next != nil {
		novel.SeriesNext = &NovelSummary{NovelRef: r.client.Novel(raw.Next.ID), Title: raw.Next.Title}
	}
	if raw.Previous != nil {
		novel.SeriesPrevious = &NovelSummary{NovelRef: r.client.Novel(raw.Previous.ID), Title: raw.Previous.Title}
	}
	return novel, nil
}

// Content returns ContentUnavailable because the App API does not provide novel bodies.
// Use ParseContent to parse webview HTML supplied by the caller; this method performs no I/O.
func (r *NovelRef) Content(ctx context.Context) (NovelContent, error) {
	if err := r.bound("Novel.Content"); err != nil {
		return NovelContent{}, err
	}
	return NovelContent{}, newError("Novel.Content", sdk.ContentUnavailable, "novel content is unsupported by the App API", nil)
}
