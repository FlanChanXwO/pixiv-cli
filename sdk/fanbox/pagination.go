package fanbox

import (
	"context"
	"net/url"
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox/internal/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk/internal/cursor"
)

func (c *Client) pageQuery(ctx context.Context, operation, path string, base url.Values, token sdk.Cursor) (url.Values, cursor.Binding, error) {
	userID, err := c.app.VerifiedUserID(ctx, operation)
	if err != nil {
		return nil, cursor.Binding{}, err
	}
	binding := cursor.Binding{Product: "fanbox", Operation: path, Account: strconv.FormatInt(userID, 10), Query: base}
	query := make(url.Values, len(base))
	for key, values := range base {
		query[key] = values
	}
	if !token.IsZero() {
		values, err := cursor.Decode(token, binding)
		if err != nil {
			return nil, binding, err
		}
		position, valid := continuationPosition(values, base)
		if !valid {
			return nil, binding, newError(operation, sdk.InvalidArgument, "invalid FANBOX cursor position")
		}
		for key, values := range position {
			query[key] = values
		}
	}
	return query, binding, nil
}

func nextCursor(raw, operation, path string, current url.Values, binding cursor.Binding) (sdk.Cursor, error) {
	if raw == "" {
		return sdk.Cursor{}, nil
	}
	target, err := url.Parse(raw)
	if err != nil || target.Scheme != "https" || target.Host != api.APIHost || target.User != nil || target.Fragment != "" || target.Path != "/"+path || target.EscapedPath() != "/"+path {
		return sdk.Cursor{}, newError(operation, sdk.MalformedUpstreamResponse, "invalid FANBOX continuation URL")
	}
	values, err := url.ParseQuery(target.RawQuery)
	if err != nil {
		return sdk.Cursor{}, newError(operation, sdk.MalformedUpstreamResponse, "invalid FANBOX continuation query")
	}
	position, valid := continuationPosition(values, binding.Query)
	if !valid {
		return sdk.Cursor{}, newError(operation, sdk.MalformedUpstreamResponse, "invalid FANBOX continuation position")
	}
	if previous, ok := continuationPosition(current, binding.Query); ok && previous.Encode() == position.Encode() {
		return sdk.Cursor{}, newError(operation, sdk.MalformedUpstreamResponse, "FANBOX continuation position did not advance")
	}
	return cursor.Encode(binding, position)
}

func continuationPosition(values, base url.Values) (url.Values, bool) {
	position := url.Values{}
	for key, entries := range values {
		if key == "" || len(entries) != 1 {
			return nil, false
		}
		if original, ok := base[key]; ok {
			if entries[0] != original[0] {
				return nil, false
			}
			continue
		}
		switch key {
		case api.ParamCreatorID, api.ParamPostID, api.ParamTag:
			return nil, false
		}
		// Paging parameters remain opaque; server echoes cannot change the query's selectors.
		position[key] = entries
	}
	return position, len(position) != 0
}
