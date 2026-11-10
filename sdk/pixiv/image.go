package pixiv

import (
	"context"
	"encoding/json"
	"io"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// Image exposes a client-bound media locator. Open and Save require that binding.
// JSON preserves the locator and dimensions, while omitting the client.
type Image struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	client *Client
}

// UnmarshalJSON clears any earlier account binding.
func (i *Image) UnmarshalJSON(data []byte) error {
	type value Image
	var decoded value
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*i = Image(decoded)
	return nil
}

// Open returns the media body; the caller must close it.
func (i Image) Open(ctx context.Context) (io.ReadCloser, error) {
	return i.client.resourceBody(ctx, "Image", i.URL)
}

// Save writes an image to path, preserving an existing file on download failure.
// The containing directory must already exist.
func (i Image) Save(ctx context.Context, path string) error {
	return i.client.saveResource(ctx, "Image", i.URL, path)
}

func (c *Client) image(raw string, width, height int) (Image, error) {
	if raw != "" {
		if err := c.app(Credentials{}).ValidateResourceURL(raw); err != nil {
			return Image{}, newError("Image", sdk.ResourceForbidden, "resource URL is not allowed", nil)
		}
	}
	return Image{URL: raw, Width: width, Height: height, client: c}, nil
}
