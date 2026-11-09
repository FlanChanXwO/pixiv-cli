package pixiv

import (
	"context"
	"net/url"
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// UserNovelsOptions resumes a user's novel list.
type UserNovelsOptions struct{ Cursor sdk.Cursor }

// Novels lists this user's novels with complete metadata and bound author references.
func (r *UserRef) Novels(ctx context.Context, options UserNovelsOptions) (sdk.Page[*Novel], error) {
	if err := r.bound("User.Novels"); err != nil {
		return sdk.Page[*Novel]{}, err
	}
	return r.client.novels(ctx, api.PathUserNovels, url.Values{api.ParamUserID: {strconv.FormatInt(r.ID, 10)}, api.ParamFilter: {"for_android"}}, options.Cursor)
}

func (c *Client) novels(ctx context.Context, path string, base url.Values, token sdk.Cursor) (sdk.Page[*Novel], error) {
	query, binding, err := c.pageQuery(ctx, path, base, token)
	if err != nil {
		return sdk.Page[*Novel]{}, err
	}
	var raw novelListWire
	if err := c.getJSON(ctx, path, query, &raw); err != nil {
		return sdk.Page[*Novel]{}, err
	}
	items, err := c.mapNovels(path, raw.Novels)
	if err != nil {
		return sdk.Page[*Novel]{}, err
	}
	next, err := nextCursor(raw.NextURL, path, base, binding)
	if err != nil {
		return sdk.Page[*Novel]{}, err
	}
	return sdk.NewPage(items, next, func(ctx context.Context) (sdk.Page[*Novel], error) { return c.novels(ctx, path, base, next) }), nil
}

func (c *Client) mapNovels(path string, list api.RequiredList[novelWire]) ([]*Novel, error) {
	if !list.Present || !list.Valid {
		return nil, newError(path, sdk.MalformedUpstreamResponse, "missing novels", nil)
	}
	items := make([]*Novel, len(list.Items))
	for i, w := range list.Items {
		if path == api.PathSearchNovel && (w.XRestrict == nil || w.TextLength == nil || w.IsOriginal == nil) {
			return nil, newError(path, sdk.MalformedUpstreamResponse, "missing novel search fields", nil)
		}
		var err error
		items[i], err = c.mapNovel(w)
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}
