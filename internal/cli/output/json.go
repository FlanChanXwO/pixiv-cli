// Package output 收纳 CLI 的通用输出适配，不包含命令或上游协议逻辑。
package output

import (
	"encoding/json"

	"github.com/FlanChanXwO/pixiv-cli/internal/record"
)

// MarshalJSONValue 保持 CLI JSON 输出的稳定 fallback：某些 SDK 零值资源的
// 默认 JSON encoder 可能拒绝编码，此时交给稳定 record codec 处理。
func MarshalJSONValue(value any, indent bool) ([]byte, error) {
	var (
		body []byte
		err  error
	)
	if indent {
		body, err = json.MarshalIndent(value, "    ", "  ")
	} else {
		body, err = json.Marshal(value)
	}
	if err == nil {
		return body, nil
	}
	return record.MarshalRecordValue(value)
}
