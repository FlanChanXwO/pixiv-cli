package pixiv

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// NovelSeriesRef addresses a novel series on its creating client.
type NovelSeriesRef struct {
	ID     int64 `json:"id"`
	client *Client
}

// NovelSeries returns a reference without network or credential I/O.
func (c *Client) NovelSeries(id int64) *NovelSeriesRef { return &NovelSeriesRef{ID: id, client: c} }

// NovelSeriesOptions resumes the same series by its upstream last_order position.
type NovelSeriesOptions struct{ Cursor sdk.Cursor }

// NovelSeries contains series metadata and one page of its novels.
// Get can resume both metadata and novels in a single request using NovelSeriesOptions.
type NovelSeries struct {
	*NovelSeriesRef
	novelSeriesData
}

type novelSeriesData struct {
	Title       string           `json:"title"`
	Caption     string           `json:"caption"`
	Author      *User            `json:"author"`
	IsConcluded bool             `json:"is_concluded"`
	Novels      sdk.Page[*Novel] `json:"novels"`
}

// UnmarshalJSON restores a series identity without retaining its earlier client.
func (r *NovelSeriesRef) UnmarshalJSON(data []byte) error {
	var value struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*r = NovelSeriesRef{ID: value.ID}
	return nil
}

// UnmarshalJSON restores metadata and novels without retaining earlier bindings.
func (s *NovelSeries) UnmarshalJSON(data []byte) error {
	var value struct {
		ID int64 `json:"id"`
		novelSeriesData
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*s = NovelSeries{NovelSeriesRef: &NovelSeriesRef{ID: value.ID}, novelSeriesData: value.novelSeriesData}
	return nil
}

// Get loads series metadata and a page of complete novels in one App request.
func (r *NovelSeriesRef) Get(ctx context.Context, options NovelSeriesOptions) (*NovelSeries, error) {
	if r == nil || r.client == nil || r.ID <= 0 {
		return nil, newError("NovelSeries.Get", sdk.InvalidArgument, "series requires a bound client and positive ID", nil)
	}
	const path = api.PathNovelSeries
	c, id := r.client, r.ID
	base := url.Values{api.ParamSeriesID: {strconv.FormatInt(id, 10)}}
	query, binding, err := c.pageQuery(ctx, path, base, options.Cursor)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Detail *struct {
			ID          int64    `json:"id"`
			Title       string   `json:"title"`
			Caption     string   `json:"caption"`
			User        userWire `json:"user"`
			IsConcluded bool     `json:"is_concluded"`
		} `json:"novel_series_detail"`
		novelListWire
	}
	if err := c.getJSON(ctx, path, query, &raw); err != nil {
		return nil, err
	}
	if raw.Detail == nil || raw.Detail.ID != id || raw.Detail.User.ID <= 0 {
		return nil, newError(path, sdk.MalformedUpstreamResponse, "invalid series detail", nil)
	}
	items, err := c.mapNovels(path, raw.Novels)
	if err != nil {
		return nil, err
	}
	author, err := c.mapUser(raw.Detail.User)
	if err != nil {
		return nil, err
	}
	next, err := nextCursor(raw.NextURL, path, base, binding)
	if err != nil {
		return nil, err
	}
	page := sdk.NewPage(items, next, func(ctx context.Context) (sdk.Page[*Novel], error) {
		series, err := c.NovelSeries(id).Get(ctx, NovelSeriesOptions{Cursor: next})
		if err != nil {
			return sdk.Page[*Novel]{}, err
		}
		return series.Novels, nil
	})
	return &NovelSeries{NovelSeriesRef: c.NovelSeries(id), novelSeriesData: novelSeriesData{Title: raw.Detail.Title, Caption: raw.Detail.Caption, Author: author, IsConcluded: raw.Detail.IsConcluded, Novels: page}}, nil
}
