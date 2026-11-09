package pixiv

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/internal/cursor"
)

const appAPIHost = "app-api.pixiv.net"

// pageQuery establishes identity before validating a persisted query/account binding.
func (c *Client) pageQuery(ctx context.Context, path string, base url.Values, token sdk.Cursor) (url.Values, cursor.Binding, error) {
	if _, err := c.credentials(ctx, Credentials{}); err != nil {
		return nil, cursor.Binding{}, err
	}
	binding := cursor.Binding{Product: "pixiv", Operation: path, Account: strconv.FormatInt(c.UserID(), 10), Query: base}
	query := cloneValues(base)
	if !token.IsZero() {
		position, err := cursor.Decode(token, binding)
		if err != nil {
			return nil, binding, err
		}
		if !validPosition(path, position, base) {
			return nil, binding, newError(path, sdk.InvalidArgument, "invalid cursor position", nil)
		}
		for key, values := range position {
			query[key] = values
		}
	}
	return query, binding, nil
}

func cloneValues(values url.Values) url.Values {
	result := make(url.Values, len(values))
	for key, entries := range values {
		result[key] = append([]string(nil), entries...)
	}
	return result
}

func validPosition(path string, values, base url.Values) bool {
	if path == api.PathSearchIllust {
		return validArtworkSearchPosition(values)
	}
	if path == api.PathIllustRelated {
		if len(values[api.ParamIllustID]) != 1 || values.Get(api.ParamIllustID) != base.Get(api.ParamIllustID) {
			return false
		}
		if _, ok := values[api.ParamOffset]; ok {
			return len(values) == 2 && positiveValues(values[api.ParamOffset], true)
		}
		return len(values) == 3 && positiveValues(values[api.ParamSeedIllustIDs], false) && positiveValues(values[api.ParamViewed], false)
	}
	key := api.ParamOffset
	if path == api.PathUserBookmarksIllust || path == api.PathUserNovelBookmarks {
		key = api.ParamMaxBookmarkID
	}
	return len(values) == 1 && positiveValues(values[key], true)
}

func positiveValues(values []string, scalar bool) bool {
	if len(values) == 0 || scalar && len(values) != 1 {
		return false
	}
	for _, text := range values {
		n, err := strconv.ParseInt(text, 10, 64)
		if err != nil || n <= 0 {
			return false
		}
	}
	return true
}

func nextCursor(raw *string, path string, base url.Values, binding cursor.Binding) (sdk.Cursor, error) {
	if raw == nil {
		return sdk.Cursor{}, nil
	}
	parsed, err := url.Parse(*raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host != appAPIHost || parsed.User != nil || parsed.Fragment != "" || parsed.Path != path || parsed.EscapedPath() != path {
		return sdk.Cursor{}, newError(path, sdk.MalformedUpstreamResponse, "invalid continuation URL", nil)
	}
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return sdk.Cursor{}, newError(path, sdk.MalformedUpstreamResponse, "invalid continuation query", nil)
	}
	position := url.Values{}
	for key, entries := range values {
		if path == api.PathSearchIllust && key == searchConsumedKey {
			return sdk.Cursor{}, newError(path, sdk.MalformedUpstreamResponse, "upstream continuation contains a local checkpoint", nil)
		}
		if len(entries) != 1 {
			return sdk.Cursor{}, newError(path, sdk.MalformedUpstreamResponse, "invalid continuation value", nil)
		}
		_, original := base[key]
		if (path != api.PathIllustRelated || key != api.ParamIllustID) && (original || (path == api.PathUserBookmarksIllust || path == api.PathUserNovelBookmarks) && key == api.ParamTag) {
			continue
		}
		position[key] = entries
	}
	if path == api.PathIllustRelated && position.Get(api.ParamOffset) == "" {
		for _, prefix := range []string{strings.TrimSuffix(api.ParamSeedIllustIDs, "]"), strings.TrimSuffix(api.ParamViewed, "]")} {
			indexed := map[int]string{}
			for key, entries := range position {
				if !strings.HasPrefix(key, prefix) {
					continue
				}
				index, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(key, prefix), "]"))
				if err != nil || index < 0 || !strings.HasSuffix(key, "]") {
					return sdk.Cursor{}, newError(path, sdk.MalformedUpstreamResponse, "invalid continuation index", nil)
				}
				if _, ok := indexed[index]; ok {
					return sdk.Cursor{}, newError(path, sdk.MalformedUpstreamResponse, "duplicate continuation index", nil)
				}
				indexed[index] = entries[0]
				delete(position, key)
			}
			if len(indexed) > 0 {
				entries := make([]string, len(indexed))
				for i := range entries {
					entries[i] = indexed[i]
				}
				position[prefix+"]"] = entries
			}
		}
	}
	if !validPosition(path, position, base) {
		return sdk.Cursor{}, newError(path, sdk.MalformedUpstreamResponse, "invalid continuation position", nil)
	}
	return cursor.Encode(binding, position)
}
