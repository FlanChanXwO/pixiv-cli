package pixiv

import (
	"context"
	"net/url"
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

const (
	searchContextKey       = "cursor_context"
	searchAIModeKey        = "ai_mode"
	searchInitialOffsetKey = "initial_offset"
	searchConsumedKey      = "consumed"
	artworkAIGenerated     = 2
)

func (c *Client) searchArtworks(ctx context.Context, base url.Values, token sdk.Cursor) (sdk.Page[*Artwork], error) {
	// 1. Validate the saved account/query position before preparing the wire query.
	query, binding, err := c.pageQuery(ctx, api.PathSearchIllust, base, token)
	if err != nil {
		return sdk.Page[*Artwork]{}, err
	}
	offset, consumed := artworkSearchPosition(query)
	for _, key := range []string{searchContextKey, searchAIModeKey, searchInitialOffsetKey, searchConsumedKey} {
		query.Del(key)
	}
	if offset > 0 {
		query.Set(api.ParamOffset, strconv.Itoa(offset))
	} else {
		query.Del(api.ParamOffset)
	}
	var raw artworkListWire
	if err := c.getJSON(ctx, api.PathSearchIllust, query, &raw); err != nil {
		return sdk.Page[*Artwork]{}, err
	}
	if !raw.Illusts.Present || !raw.Illusts.Valid {
		return sdk.Page[*Artwork]{}, newError("SearchArtworks", sdk.MalformedUpstreamResponse, "missing artworks", nil)
	}
	// 2. Normalize the complete batch, then apply AI-only selection and its checkpoint.
	items := make([]*Artwork, 0, len(raw.Illusts.Items))
	for _, wire := range raw.Illusts.Items {
		artwork, err := c.mapArtwork(wire)
		if err != nil {
			return sdk.Page[*Artwork]{}, err
		}
		if base.Get(searchAIModeKey) == string(SearchAIModeOnly) && artwork.AIType != artworkAIGenerated {
			continue
		}
		items = append(items, artwork)
	}
	if consumed > len(items) {
		return sdk.Page[*Artwork]{}, newError("SearchArtworks", sdk.InvalidArgument, "checkpoint exceeds the current batch; restart pagination", nil)
	}
	next, err := nextCursor(raw.NextURL, api.PathSearchIllust, base, binding)
	if err != nil {
		return sdk.Page[*Artwork]{}, err
	}
	return sdk.NewPage(items[consumed:], next, func(ctx context.Context) (sdk.Page[*Artwork], error) { return c.searchArtworks(ctx, base, next) }), nil
}

// artworkSearchPosition reads integers already checked by pageQuery or constructed from validated options.
func artworkSearchPosition(query url.Values) (offset, consumed int) {
	if value := query.Get(api.ParamOffset); value != "" {
		offset, _ = strconv.Atoi(value)
	} else if value := query.Get(searchInitialOffsetKey); value != "" {
		offset, _ = strconv.Atoi(value)
	}
	if value := query.Get(searchConsumedKey); value != "" {
		consumed, _ = strconv.Atoi(value)
	}
	return offset, consumed
}

func validArtworkSearchPosition(values url.Values) bool {
	if len(values[api.ParamOffset]) != 1 {
		return false
	}
	offset, err := strconv.Atoi(values.Get(api.ParamOffset))
	if err != nil || offset < 0 {
		return false
	}
	if len(values) == 1 {
		return offset > 0
	}
	if len(values) != 2 || len(values[searchConsumedKey]) != 1 {
		return false
	}
	consumed, err := strconv.Atoi(values.Get(searchConsumedKey))
	return err == nil && consumed > 0
}
