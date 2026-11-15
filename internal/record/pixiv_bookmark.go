package record

import (
	"encoding/json"

	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// BookmarkDetail 保留 CLI 收藏详情字段与空标签的 null 输出。
type BookmarkDetail struct {
	Restrict pixiv.Restrict `json:"restrict"`
	Tags     []string       `json:"tags"`
}

// MarshalJSON 保留空标签的既有输出。
func (value BookmarkDetail) MarshalJSON() ([]byte, error) {
	if len(value.Tags) == 0 {
		value.Tags = nil
	}
	type output BookmarkDetail
	return json.Marshal(output(value))
}
