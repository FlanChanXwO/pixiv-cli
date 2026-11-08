package pixiv

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
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
	response, err := i.client.openResource(ctx, i.URL)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, classifyError("Image.Open", api.HTTPStatus(response.StatusCode), false)
	}
	return response.Body, nil
}

// Save writes an image to path, preserving an existing file on download failure.
// The containing directory must already exist.
func (i Image) Save(ctx context.Context, path string) error {
	body, err := i.Open(ctx)
	if err != nil {
		return err
	}
	defer body.Close()
	file, err := os.CreateTemp(filepath.Dir(path), ".pixiv-image-*")
	if err != nil {
		return newError("Image.Save", sdk.LocalStateError, "create image file failed", err)
	}
	name := file.Name()
	defer os.Remove(name)
	_, copyErr := io.Copy(file, body)
	closeErr := file.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return newError("Image.Save", sdk.LocalStateError, "write image failed", err)
	}
	if err := os.Rename(name, path); err != nil {
		return newError("Image.Save", sdk.LocalStateError, "replace image failed", err)
	}
	return nil
}

func (c *Client) image(raw string, width, height int) (Image, error) {
	if raw != "" {
		if err := c.app(Credentials{}).ValidateResourceURL(raw); err != nil {
			return Image{}, newError("Image", sdk.ResourceForbidden, "resource URL is not allowed", nil)
		}
	}
	return Image{URL: raw, Width: width, Height: height, client: c}, nil
}
