package pixiv

import (
	"context"
	"encoding/json"
	"io"
)

// File exposes a client-bound novel attachment; Size is bytes, or zero when absent.
type File struct {
	URL      string `json:"url"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	client   *Client
}

// UnmarshalJSON restores attachment metadata and clears the earlier account binding.
func (f *File) UnmarshalJSON(data []byte) error {
	type value File
	var decoded value
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*f = File(decoded)
	return nil
}

// Open returns an attachment body; the caller must close it.
func (f File) Open(ctx context.Context) (io.ReadCloser, error) {
	return f.client.resourceBody(ctx, "File", f.URL)
}

// Save writes an attachment, preserving an existing destination on download failure.
// The containing directory must already exist.
func (f File) Save(ctx context.Context, path string) error {
	return f.client.saveResource(ctx, "File", f.URL, path)
}
