package fanbox

import (
	"encoding/json"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox/internal/api"
)

// Image carries a first-party media locator and its optional pixel dimensions.
// JSON preserves metadata and omits the account binding.
type Image struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	client *Client
}

// UnmarshalJSON restores image metadata and clears the earlier account binding.
func (i *Image) UnmarshalJSON(data []byte) error {
	type value Image
	var decoded value
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*i = Image(decoded)
	return nil
}

func (c *Client) image(operation, raw string) (Image, error) {
	raw = strings.TrimSpace(raw)
	if raw != "" {
		if api.ValidateMediaURL(raw) != nil {
			return Image{}, newError(operation, sdk.ResourceForbidden, "FANBOX image URL is not allowed")
		}
	}
	return Image{URL: raw, client: c}, nil
}
