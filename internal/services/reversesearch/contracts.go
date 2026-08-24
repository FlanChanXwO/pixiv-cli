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

// Quota 是 provider 可安全公开的剩余额度摘要，不包含凭据或账号标识。
type Quota struct {
	ShortRemaining int `json:"short_remaining"`
	LongRemaining  int `json:"long_remaining"`
	ShortLimit     int `json:"short_limit"`
	LongLimit      int `json:"long_limit"`
}

// Match 是单个 provider 的原始领域命中。Canonical Pixiv 去重与 evidence
// 聚合属于顶层 aggregator，而不是协议 adapter。
type Match struct {
	Rank         int      `json:"rank"`
	Similarity   float64  `json:"similarity"`
	IndexID      int      `json:"index_id"`
	IndexName    string   `json:"index_name"`
	Title        string   `json:"title,omitempty"`
	Author       string   `json:"author,omitempty"`
	ArtworkID    int64    `json:"artwork_id,omitempty"`
	UserID       int64    `json:"user_id,omitempty"`
	ExternalURLs []string `json:"external_urls,omitempty"`
}

// ProviderResponse 是一次 provider 查询的领域结果。
type ProviderResponse struct {
	Provider Provider `json:"provider"`
	Matches  []Match  `json:"matches"`
	Quota    *Quota   `json:"quota,omitempty"`
}

// ProviderClient 是 aggregator 注入的 provider 端口。
type ProviderClient interface {
	Preflight(context.Context) error
	Search(context.Context, *Snapshot) (ProviderResponse, error)
}

// ASCII2DClient 是 aggregator 注入的 ascii2d 上传端口。一次上传返回的
// session 可供 color 与 bovw 并发查询，避免重复外传同一图片。
type ASCII2DClient interface {
	Preflight(context.Context) error
	Upload(context.Context, *Snapshot) (ASCII2DSession, error)
}

// ASCII2DSession 是一次 ascii2d 上传产生的查询端口。
type ASCII2DSession interface {
	Search(context.Context, Provider) (ProviderResponse, error)
}

// Searcher 是 CLI/MCP 依赖的反向搜图端口。
type Searcher interface {
	Search(context.Context, Request) (Response, error)
}
