package fanbox

import (
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
