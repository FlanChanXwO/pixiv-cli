package pixiv

import (
	"context"
	"encoding/json"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// TrendingTag pairs a translated tag with its bound sample artwork.
type TrendingTag struct {
	Tag     Tag      `json:"tag"`
	Artwork *Artwork `json:"artwork"`
}

// UnmarshalJSON restores a tag and artwork without retaining earlier bindings.
func (t *TrendingTag) UnmarshalJSON(data []byte) error {
	type plain TrendingTag
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*t = TrendingTag(value)
	return nil
}

// TrendingArtworkTags lists current artwork tags with translations and sample artworks.
func (c *Client) TrendingArtworkTags(ctx context.Context) ([]TrendingTag, error) {
	const path = api.PathTrendingTagsIllust
	var raw struct {
		Tags api.RequiredList[struct {
			Tag            string                          `json:"tag"`
			TranslatedName string                          `json:"translated_name"`
			Artwork        api.RequiredObject[artworkWire] `json:"illust"`
		}] `json:"trend_tags"`
	}
	if err := c.getJSON(ctx, path, nil, &raw); err != nil {
		return nil, err
	}
	if !raw.Tags.Present || !raw.Tags.Valid {
		return nil, newError(path, sdk.MalformedUpstreamResponse, "missing trending tags", nil)
	}
	result := make([]TrendingTag, len(raw.Tags.Items))
	for i, w := range raw.Tags.Items {
		if w.Tag == "" || !w.Artwork.Present || !w.Artwork.Valid {
			return nil, newError(path, sdk.MalformedUpstreamResponse, "invalid trending tag", nil)
		}
		artwork, err := c.mapArtwork(w.Artwork.Value)
		if err != nil {
			return nil, err
		}
		result[i] = TrendingTag{Tag: Tag{Name: w.Tag, TranslatedName: w.TranslatedName}, Artwork: artwork}
	}
	return result, nil
}
