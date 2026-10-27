package mediaproxy

import (
	"time"
)

// AnimationStatus 只含控制信息与受保护的媒体 URL，不含二进制内容。
type AnimationStatus struct {
	// FrameCount 仅供 UI 缓存预算，不进入公开结构化输出。
	FrameCount   int       `json:"-"`
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
