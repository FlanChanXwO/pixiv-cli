package pixiv

import (
	"context"
	"encoding/json"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// Stamp pairs an upstream stamp ID with its client-bound image.
// Use its ID in CommentOptions to post it as a comment.
type Stamp struct {
	ID    int64 `json:"id"`
	Image Image `json:"image"`
}

type stampWire struct {
	ID  int64  `json:"stamp_id"`
	URL string `json:"stamp_url"`
}

func (c *Client) mapStamp(w stampWire) (Stamp, error) {
	if w.ID <= 0 || !api.ValidStampURL(w.URL) {
		return Stamp{}, newError("Stamp", sdk.MalformedUpstreamResponse, "invalid stamp ID or URL", nil)
	}
	return Stamp{ID: w.ID, Image: Image{URL: w.URL, client: c}}, nil
}

// UnmarshalJSON restores stamp data without retaining an earlier image binding.
func (s *Stamp) UnmarshalJSON(data []byte) error {
	type value Stamp
	var decoded value
	if err := json.Unmarshal(data, &decoded); err != nil {
		return newError("Stamp.UnmarshalJSON", sdk.InvalidArgument, "malformed stamp", err)
	}
	*s = Stamp(decoded)
	return nil
}

// Stamps lists the available comment stamps; upstream provides no continuation.
func (c *Client) Stamps(ctx context.Context) ([]Stamp, error) {
	var raw struct {
		Stamps  api.RequiredList[stampWire] `json:"stamps"`
		NextURL *json.RawMessage            `json:"next_url"`
	}
	if err := c.getJSON(ctx, api.PathStamps, nil, &raw); err != nil {
		return nil, err
	}
	if !raw.Stamps.Present || !raw.Stamps.Valid || raw.NextURL != nil {
		return nil, newError("Stamps", sdk.MalformedUpstreamResponse, "invalid stamp list", nil)
	}
	items := make([]Stamp, len(raw.Stamps.Items))
	for i, w := range raw.Stamps.Items {
		stamp, err := c.mapStamp(w)
		if err != nil {
			return nil, err
		}
		items[i] = stamp
	}
	return items, nil
}
