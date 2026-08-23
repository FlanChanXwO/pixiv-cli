// Package reversesearch 提供反向搜图的领域契约、载荷生命周期与 provider 编排。
// 协议适配器位于子包；CLI/MCP 只依赖本顶层包。
package reversesearch

import "context"

// Provider 标识一次查询选择的上游能力。
type Provider string

const (
	ProviderSauceNAO     Provider = "saucenao"
	ProviderASCII2DColor Provider = "ascii2d-color"
	ProviderASCII2DBOVW  Provider = "ascii2d-bovw"
	ProviderAll          Provider = "all"
)

// SourceKind 是已经载入本地快照的原始来源类别，不携带 source 本身。
type SourceKind string

const (
	SourceKindFile SourceKind = "file"
	SourceKindURL  SourceKind = "url"
)

// Request 是一次反向搜图请求。凭据、HTTP client 和代理均为构造依赖。
type Request struct {
	Source    string
	Provider  Provider
	PixivOnly bool
}

// Input 是可安全进入 JSON/MCP 输出的输入摘要。
type Input struct {
	Kind   SourceKind `json:"kind"`
	SHA256 string     `json:"sha256"`
}

// Response 是 service 层的领域响应。provider 结果与聚合字段将在对应实现
// task 中扩展；它不依赖 CLI/MCP 的 Record 类型。
type Response struct {
	Input Input `json:"input"`
}

// Searcher 是 CLI/MCP 依赖的反向搜图端口。
type Searcher interface {
	Search(context.Context, Request) (Response, error)
}
