package record

import (
	"encoding/base64"
	"encoding/json"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// Resource 只交付稳定引用与凭据需求，资源地址保留在 SDK 绑定对象内。
type Resource struct {
	Ref                 ResourceRef `json:"ref"`
	RequiresCredentials bool        `json:"requires_credentials,omitempty"`
}

// MarshalJSON 保留缺失资源的 null 输出。
func (r Resource) MarshalJSON() ([]byte, error) {
	if r.Ref.IsZero() {
		return []byte("null"), nil
	}
	type output Resource
	return json.Marshal(output(r))
}

type resourceEnvelope struct {
	Version int    `json:"v"`
	Product string `json:"p"`
	Payload []byte `json:"d"`
}

// ResourceRef 保留 CLI/MCP 的版本化媒体身份编码。
type ResourceRef struct {
	text    string
	product string
	payload []byte
}

func newResourceRef(product string, payload []byte) (ResourceRef, error) {
	encoded, err := json.Marshal(resourceEnvelope{Version: 1, Product: product, Payload: payload})
	if err != nil {
		return ResourceRef{}, err
	}
	return ResourceRef{text: base64.RawURLEncoding.EncodeToString(encoded), product: product, payload: payload}, nil
}

// IsZero 报告资源引用是否未设置。
func (r ResourceRef) IsZero() bool { return r.text == "" }

// String 返回可放在路由中的稳定引用。
func (r ResourceRef) String() string { return r.text }

// Product 返回已经解析的产品命名空间。
func (r ResourceRef) Product() string { return r.product }

// Payload 返回已经解析的产品身份载荷。
func (r ResourceRef) Payload() []byte { return r.payload }

// MarshalJSON 把资源引用编码为字符串。
func (r ResourceRef) MarshalJSON() ([]byte, error) { return json.Marshal(r.text) }

// ParseResourceRef 在媒体输入边界校验版本与必要身份字段。
func ParseResourceRef(text string) (ResourceRef, error) {
	raw, err := base64.RawURLEncoding.DecodeString(text)
	if err != nil {
		return ResourceRef{}, &sdk.Error{Operation: "ParseResourceRef", Reason: sdk.InvalidArgument, Err: err}
	}
	var envelope resourceEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return ResourceRef{}, &sdk.Error{Operation: "ParseResourceRef", Reason: sdk.InvalidArgument, Message: "malformed reference envelope"}
	}
	if envelope.Version != 1 {
		return ResourceRef{}, &sdk.Error{Operation: "ParseResourceRef", Reason: sdk.InvalidArgument, Message: "unsupported reference format version"}
	}
	if envelope.Product == "" || len(envelope.Payload) == 0 {
		return ResourceRef{}, &sdk.Error{Operation: "ParseResourceRef", Reason: sdk.InvalidArgument, Message: "reference missing product or payload"}
	}
	return ResourceRef{text: text, product: envelope.Product, payload: envelope.Payload}, nil
}
