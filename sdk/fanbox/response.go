package fanbox

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

func (c *Client) getBody(ctx context.Context, operation, path string, query url.Values) (json.RawMessage, error) {
	var response struct {
		Body json.RawMessage `json:"body"`
	}
	if err := c.app.GetJSON(ctx, operation, path, query, &response); err != nil {
		return nil, err
	}
	if len(response.Body) == 0 || strings.TrimSpace(string(response.Body)) == "null" {
		return nil, newError(operation, sdk.MalformedUpstreamResponse, "missing FANBOX response body")
	}
	return response.Body, nil
}

type paginationWire struct {
	NextURL  string   `json:"nextUrl"`
	PageURLs []string `json:"pageUrls"`
}

func (w paginationWire) continuation() string {
	if len(w.PageURLs) != 0 {
		return w.PageURLs[0]
	}
	return w.NextURL
}
