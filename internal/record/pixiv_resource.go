package record

import (
	"encoding/json"

	legacysdk "github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk"
)

// outputResource 保留旧媒体工具消费的身份格式，locators 留在 SDK 绑定对象内。
func outputResource(kind string, id int64, page int, variant string) (legacysdk.Resource, error) {
	payload, err := json.Marshal(struct {
		Kind    string `json:"k"`
		ID      int64  `json:"id"`
		Page    int    `json:"p,omitempty"`
		Variant string `json:"v,omitempty"`
	}{kind, id, page, variant})
	if err != nil {
		return legacysdk.Resource{}, err
	}
	ref, err := legacysdk.NewResourceRef("pixiv", payload)
	return legacysdk.Resource{Ref: ref}, err
}
