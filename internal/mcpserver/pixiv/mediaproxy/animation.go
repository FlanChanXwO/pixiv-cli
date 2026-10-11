package mediaproxy

import (
	"time"
)

// AnimationStatus contains control data only; binaries use protected media URLs.
type AnimationStatus struct {
	Handle       string    `json:"handle,omitempty"`
	State        string    `json:"state"`
	AnimationURL string    `json:"animation_url,omitempty"`
	PreviewURL   string    `json:"preview_url,omitempty"`
	MediaOrigin  string    `json:"media_origin,omitempty"`
	MIMEType     string    `json:"mime_type,omitempty"`
	Filename     string    `json:"filename,omitempty"`
	Size         int64     `json:"size,omitempty"`
	Expires      time.Time `json:"expires,omitempty"`
	Error        string    `json:"error,omitempty"`
}
