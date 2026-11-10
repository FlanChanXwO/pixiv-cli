package fanbox

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox/internal/api"
)

// File carries a first-party attachment locator and its upstream filename.
// JSON preserves metadata and omits the account binding.
type File struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Extension string `json:"extension"`
	URL       string `json:"url"`
	client    *Client
}

// Open streams this attachment with optional GET/HEAD conditions; the caller closes Body.
func (f File) Open(ctx context.Context, request OpenRequest) (*MediaResponse, error) {
	response, err := f.client.openMedia(ctx, "File.Open", f.URL, request)
	if err != nil {
		return nil, err
	}
	response.Filename = f.Filename()
	return response, nil
}

// Save atomically writes this attachment to path, creating its containing directory.
// Download, cancellation and close failures preserve an existing destination.
func (f File) Save(ctx context.Context, path string) error {
	return f.client.saveMedia(ctx, "File.Save", f.URL, path)
}

// Filename returns the upstream name with its extension appended at most once,
// case-insensitively. Empty names remain empty; the value is untrusted metadata.
func (f File) Filename() string {
	if f.Name == "" || f.Extension == "" || strings.HasSuffix(strings.ToLower(f.Name), "."+strings.ToLower(f.Extension)) {
		return f.Name
	}
	return f.Name + "." + f.Extension
}

// UnmarshalJSON restores file metadata and clears the earlier account binding.
func (f *File) UnmarshalJSON(data []byte) error {
	type value File
	var decoded value
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*f = File(decoded)
	return nil
}

func (c *Client) postFile(operation string, source fileWire) (File, error) {
	if strings.TrimSpace(source.ID) == "" {
		return File{}, newError(operation, sdk.MalformedUpstreamResponse, "FANBOX file is missing an ID")
	}
	raw := strings.TrimSpace(source.URL)
	if api.ValidateMediaURL(raw) != nil {
		return File{}, newError(operation, sdk.ResourceForbidden, "FANBOX file URL is not allowed")
	}
	return File{ID: source.ID, Name: source.Name, Extension: source.Extension, URL: raw, client: c}, nil
}
