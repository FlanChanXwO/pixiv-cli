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

// Response 是 service 层的稳定领域 envelope；它不依赖 CLI/MCP 的 Record 类型。
type Response struct {
	Input          Input             `json:"input"`
	Providers      []ProviderSummary `json:"providers"`
	Results        []Result          `json:"results"`
	ProviderErrors []ProviderError   `json:"provider_errors"`
	Partial        bool              `json:"partial"`
}

// ProviderStatus 描述 provider 是否产生了可用响应。
type ProviderStatus string

const (
	ProviderStatusSuccess ProviderStatus = "success"
	ProviderStatusError   ProviderStatus = "error"
)

// ProviderSummary 是按请求固定顺序发布的 provider 执行摘要。
type ProviderSummary struct {
	Name        Provider       `json:"name"`
	Status      ProviderStatus `json:"status"`
	ResultCount int            `json:"result_count"`
	Quota       *Quota         `json:"quota,omitempty"`
}

// ProviderError 是可安全跨 CLI/MCP 边界发布的 provider 错误。
type ProviderError struct {
	Provider Provider  `json:"provider"`
	Code     ErrorCode `json:"code"`
	Message  string    `json:"message"`
}

// PixivRefType 是严格识别出的 Pixiv canonical identity 类型。
type PixivRefType string

const (
	PixivRefArtwork PixivRefType = "artwork"
	PixivRefUser    PixivRefType = "user"
)

// PixivRef 是不猜测 artwork subtype 的 canonical Pixiv identity。
type PixivRef struct {
	Type PixivRefType `json:"type"`
	ID   int64        `json:"id"`
}

// Evidence 保留单个 provider/rank 的原始证据；跨 provider 分数不换算。
type Evidence struct {
	Provider     Provider `json:"provider"`
	Rank         int      `json:"rank"`
	Similarity   float64  `json:"similarity"`
	IndexID      int      `json:"index_id"`
	IndexName    string   `json:"index_name"`
	Title        string   `json:"title,omitempty"`
	Author       string   `json:"author,omitempty"`
	ExternalURLs []string `json:"external_urls,omitempty"`
}

// Result 是一个可选 canonical Pixiv identity 及其一个或多个 provider 证据。
type Result struct {
	Pixiv    *PixivRef  `json:"pixiv,omitempty"`
	Title    string     `json:"title,omitempty"`
	Author   string     `json:"author,omitempty"`
	Evidence []Evidence `json:"evidence"`
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
