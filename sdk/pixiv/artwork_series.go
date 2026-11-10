package pixiv

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// ArtworkSeriesRef addresses an illustration series on its creating client.
// OwnerUserID is populated when the source URL or artwork names the owner;
// listing needs only ID, while CanonicalURL also requires OwnerUserID.
type ArtworkSeriesRef struct {
	ID          int64 `json:"id"`
	OwnerUserID int64 `json:"owner_user_id,omitempty"`
	client      *Client
}

// ArtworkSeries returns a series reference without network or credential I/O.
func (c *Client) ArtworkSeries(id int64) *ArtworkSeriesRef {
	return &ArtworkSeriesRef{ID: id, client: c}
}

// ArtworkSeriesOptions resumes the same series by its upstream continuation.
type ArtworkSeriesOptions struct{ Cursor sdk.Cursor }

// Artworks lists complete artworks in this series.
func (r *ArtworkSeriesRef) Artworks(ctx context.Context, options ArtworkSeriesOptions) (sdk.Page[*Artwork], error) {
	if r == nil || r.client == nil || r.ID <= 0 {
		return sdk.Page[*Artwork]{}, newError("ArtworkSeries.Artworks", sdk.InvalidArgument, "series requires a bound client and positive ID", nil)
	}
	return r.client.artworks(ctx, api.PathIllustSeries, url.Values{api.ParamIllustSeriesID: {strconv.FormatInt(r.ID, 10)}}, options.Cursor)
}

// UnmarshalJSON restores the series identity without retaining an earlier client.
func (r *ArtworkSeriesRef) UnmarshalJSON(data []byte) error {
	var value struct {
		ID          int64 `json:"id"`
		OwnerUserID int64 `json:"owner_user_id"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*r = ArtworkSeriesRef{ID: value.ID, OwnerUserID: value.OwnerUserID}
	return nil
}

// UnmarshalJSON restores a summary and clears all earlier client bindings.
func (s *ArtworkSeriesSummary) UnmarshalJSON(data []byte) error {
	var value struct {
		ID          int64  `json:"id"`
		OwnerUserID int64  `json:"owner_user_id"`
		Title       string `json:"title"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*s = ArtworkSeriesSummary{ArtworkSeriesRef: &ArtworkSeriesRef{ID: value.ID, OwnerUserID: value.OwnerUserID}, Title: value.Title}
	return nil
}
