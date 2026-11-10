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
	ID        string `json:"id"`
	Extension string `json:"extension"`
	URL       string `json:"url"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Thumbnail *Image `json:"thumbnail,omitempty"`
	client    *Client
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

func (c *Client) postImage(operation string, source imageWire) (Image, error) {
	if strings.TrimSpace(source.ID) == "" {
		return Image{}, newError(operation, sdk.MalformedUpstreamResponse, "FANBOX image is missing an ID")
	}
	if strings.TrimSpace(source.OriginalURL) == "" {
		return Image{}, newError(operation, sdk.ResourceForbidden, "FANBOX image URL is required")
	}
	image, err := c.image(operation, source.OriginalURL)
	if err != nil {
		return Image{}, err
	}
	image.ID, image.Extension, image.Width, image.Height = source.ID, source.Extension, source.Width, source.Height
	if source.ThumbnailURL != "" {
		thumbnail, err := c.image(operation, source.ThumbnailURL)
		if err != nil {
			return Image{}, err
		}
		if thumbnail.URL == "" {
			return Image{}, newError(operation, sdk.ResourceForbidden, "FANBOX thumbnail URL is required")
		}
		image.Thumbnail = &thumbnail
	}
	return image, nil
}
