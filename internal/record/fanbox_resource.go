package record

import (
	"encoding/json"
	"net/url"
	"strings"
)

// FanboxResourceRole 标识公开资源引用的媒体角色。
type FanboxResourceRole string

const (
	// FanboxCreatorIcon 标识创作者头像。
	FanboxCreatorIcon FanboxResourceRole = "creator_icon"
	// FanboxCreatorCover 标识创作者封面。
	FanboxCreatorCover FanboxResourceRole = "creator_cover"
	// FanboxPostImage 标识帖子图片。
	FanboxPostImage FanboxResourceRole = "post_image"
	// FanboxPostFile 标识帖子附件。
	FanboxPostFile FanboxResourceRole = "post_file"
)

// FanboxResourceIdentity 保持已有资源引用载荷；地址、签名与凭据不进入载荷。
type FanboxResourceIdentity struct {
	Role    FanboxResourceRole `json:"k"`
	Creator string             `json:"c,omitempty"`
	Post    string             `json:"p,omitempty"`
	Asset   string             `json:"a,omitempty"`
}

// FanboxImageResource 将已验证的图片地址投影为稳定资源身份；URL 不进入输出。
func FanboxImageResource(role FanboxResourceRole, creator, post, asset, locator string) Resource {
	if locator == "" {
		return Resource{}
	}
	// 此身份只含字符串；固定产品名与非空 JSON 满足通用引用编码器的前置条件。
	payload, _ := json.Marshal(FanboxResourceIdentity{role, creator, post, asset})
	ref, _ := newResourceRef("fanbox", payload)
	parsed, _ := url.Parse(locator)
	return Resource{Ref: ref, RequiresCredentials: strings.EqualFold(parsed.Hostname(), "downloads.fanbox.cc")}
}
