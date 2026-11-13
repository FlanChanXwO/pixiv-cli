package pixiv

import (
	"context"
	"encoding/json"
	"io"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// Image exposes a client-bound media locator. Open and Save require that binding.
// JSON preserves the locator and dimensions, while omitting the client.
type Image struct {
	URL     string `json:"url"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	client  *Client
	variant string
}

// Variant reports the upstream size role selected for a cover or original image.
// It is runtime metadata and is empty after JSON decoding.
func (i Image) Variant() string { return i.variant }

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

func (c *Client) coverImage(w imageURLsWire, width, height int) (Image, error) {
	for _, candidate := range []struct{ variant, url string }{
		{"original", w.Original}, {"large", w.Large}, {"medium", w.Medium}, {"square_medium", w.SquareMedium},
	} {
		if candidate.url != "" {
			image, err := c.image(candidate.url, width, height)
			image.variant = candidate.variant
			return image, err
		}
	}
	return c.image("", width, height)
}

// Image constructs a client-bound image for an allowed CDN URL without I/O.
// Open and Save retain the client's credential and resource policies.
func (c *Client) Image(rawURL string) (Image, error) {
	return c.image(rawURL, 0, 0)
}

// Quality selects a derived artwork size while retaining the account binding.
// Original returns the image unchanged; other sizes require an original artwork URL.
func (i Image) Quality(quality string) (Image, error) {
	if quality == "" || quality == "original" {
		return i, nil
	}
	original, err := url.Parse(i.URL)
	if err != nil {
		return Image{}, newError("Image.Quality", sdk.InvalidArgument, "invalid original image URL", nil)
	}
	_, relative, ok := strings.Cut(original.Path, "/img-original/img/")
	if !ok || original.Scheme != "https" {
		return Image{}, newError("Image.Quality", sdk.InvalidArgument, "image quality variant is unavailable", nil)
	}
	var crop, suffix string
	switch quality {
	case "regular":
		crop, suffix = "1200x1200", "_master1200"
	case "small":
		crop, suffix = "540x540_70", "_master1200"
	case "thumb", "square_medium":
		crop, suffix = "250x250_80_a2", "_square1200"
	case "mini":
		crop, suffix = "48x48", "_square1200"
	default:
		return Image{}, newError("Image.Quality", sdk.InvalidArgument, "image quality variant is unavailable", nil)
	}
	original.Path = "/c/" + crop + "/img-master/img/" + strings.TrimSuffix(relative, path.Ext(relative)) + suffix + ".jpg"
	original.RawPath, original.RawQuery, original.Fragment = "", "", ""
	i.variant = quality
	i.URL = original.String()
	return i, nil
}

// ImageResponse exposes media metadata without credentials or a resolved URL.
// The caller owns Body, including for a non-success StatusCode.
type ImageResponse struct {
	StatusCode    int
	ContentType   string
	ContentLength int64 // -1 when the upstream did not declare a valid length.
	Body          io.ReadCloser
}

// OpenResponse opens the same policy-checked stream as Open and retains its HTTP
// status and media headers for streaming proxies and download filename selection.
func (i Image) OpenResponse(ctx context.Context) (*ImageResponse, error) {
	response, err := i.client.openResource(ctx, i.URL)
	if err != nil {
		return nil, err
	}
	length := int64(-1)
	if raw := response.Header.Get("Content-Length"); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n >= 0 {
			length = n
		}
	}
	return &ImageResponse{StatusCode: response.StatusCode, ContentType: response.Header.Get("Content-Type"), ContentLength: length, Body: response.Body}, nil
}
