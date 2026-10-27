// Package reversesearch 提供反向搜图的领域契约、载荷生命周期与 provider 编排。
//
// 协议适配器位于子包；CLI/MCP 只依赖本顶层包，provider 的传输细节不越过该边界。
//
// 包的组成：[Service] 固化 source 并返回 [Response]，[Loader] 与 [Snapshot] 管理
// 私有载荷，[Aggregator] 并发编排 provider，[Error] 携带稳定 [ErrorCode]；
// [ProviderClient]、[ASCII2DClient] 是 provider 端口。
package reversesearch

import (
	"context"
	"errors"
)

// Provider 标识一次查询选择的上游能力。
type Provider string

// ProviderSauceNAO、ProviderASCII2DColor、ProviderASCII2DBOVW 与 ProviderAll 是请求
// 可选择的 provider；all 会并发查询前三者并按固定顺序返回摘要。
const (
	ProviderSauceNAO     Provider = "saucenao"
	ProviderASCII2DColor Provider = "ascii2d-color"
	ProviderASCII2DBOVW  Provider = "ascii2d-bovw"
	ProviderAll          Provider = "all"
)

// SourceKind 枚举已经载入本地快照的原始来源类别，不携带 source 本身。
type SourceKind string

// SourceKindFile 与 SourceKindURL 区分快照来自本地文件还是 HTTP(S) URL。
const (
	SourceKindFile SourceKind = "file"
	SourceKindURL  SourceKind = "url"
)

// Request 保存一次反向搜图请求。凭据、HTTP client 和代理均为构造依赖。
type Request struct {
	Source    string
	Provider  Provider
	PixivOnly bool
}

// Input 汇总可安全进入 JSON/MCP 输出的输入。
type Input struct {
	Kind   SourceKind `json:"kind"`
	SHA256 string     `json:"sha256"`
}

// Response 携带 service 层的稳定领域 envelope；它不依赖 CLI/MCP 的 Record 类型。
type Response struct {
	Input          Input             `json:"input"`
	Providers      []ProviderSummary `json:"providers"`
	Results        []Result          `json:"results"`
	ProviderErrors []ProviderError   `json:"provider_errors"`
	Partial        bool              `json:"partial"`
}

// ProviderStatus 描述 provider 是否产生了可用响应。
type ProviderStatus string

// ProviderStatusSuccess 表示 provider 产生了可用响应，ProviderStatusError 表示该 provider
// 在 partial 结果中失败。
const (
	ProviderStatusSuccess ProviderStatus = "success"
	ProviderStatusError   ProviderStatus = "error"
)

// ProviderSummary 汇总按请求固定顺序发布的 provider 执行摘要。
type ProviderSummary struct {
	Name        Provider       `json:"name"`
	Status      ProviderStatus `json:"status"`
	ResultCount int            `json:"result_count"`
	Quota       *Quota         `json:"quota,omitempty"`
}

// ProviderError 携带可安全跨 CLI/MCP 边界发布的 provider 错误。
type ProviderError struct {
	Provider Provider  `json:"provider"`
	Code     ErrorCode `json:"code"`
	Message  string    `json:"message"`
}

// PixivRefType 枚举严格识别出的 Pixiv canonical identity 类型。
type PixivRefType string

// PixivRefArtwork 与 PixivRefUser 是能严格识别的 Pixiv 身份类型。
const (
	PixivRefArtwork PixivRefType = "artwork"
	PixivRefUser    PixivRefType = "user"
)

// PixivRef 保存严格识别出的 canonical Pixiv identity，不猜测 artwork subtype。
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

// Result 汇总一个可选 canonical Pixiv identity 及其一个或多个 provider 证据。
type Result struct {
	Pixiv    *PixivRef  `json:"pixiv,omitempty"`
	Title    string     `json:"title,omitempty"`
	Author   string     `json:"author,omitempty"`
	Evidence []Evidence `json:"evidence"`
}

// Quota 表示 provider 可安全公开的剩余额度摘要，不包含凭据或账号标识。
type Quota struct {
	ShortRemaining int `json:"short_remaining"`
	LongRemaining  int `json:"long_remaining"`
	ShortLimit     int `json:"short_limit"`
	LongLimit      int `json:"long_limit"`
}

// Match 记录单个 provider 的原始领域命中。Canonical Pixiv 去重与 evidence
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

// ProviderResponse 携带一次 provider 查询的领域结果；其数据所有权交给 aggregator，
// 后者可按 rank 排序并将 evidence、quota 直接交给最终响应。
type ProviderResponse struct {
	Matches []Match `json:"matches"`
	Quota   *Quota  `json:"quota,omitempty"`
}

// ProviderClient 定义 aggregator 注入的 provider 端口。
type ProviderClient interface {
	Closer
	Preflight(context.Context) error
	Search(context.Context, *Snapshot) (ProviderResponse, error)
}

// ASCII2DClient 定义 aggregator 注入的 ascii2d 上传端口。一次上传返回的
// session 可供 color 与 bovw 并发查询，避免重复外传同一图片。
type ASCII2DClient interface {
	Closer
	Upload(context.Context, *Snapshot) (ASCII2DSession, error)
}

// ASCII2DSession 定义一次 ascii2d 上传产生的查询端口。
type ASCII2DSession interface {
	Search(context.Context, Provider) (ProviderResponse, error)
}

// Searcher 定义 CLI/MCP 依赖的反向搜图端口。
type Searcher interface {
	Search(context.Context, Request) (Response, error)
}

// Closer 由命令组合根注册，释放本次调用拥有的反搜资源。
type Closer interface {
	Close() error
}

// Service 将不透明 source 固化一次，并确保 provider 查询结束后清理快照。
type Service struct {
	sources  *Loader
	payloads *Aggregator
}

// NewService 组合 source 载入与 provider 编排；两个依赖都由调用方负责关闭。
func NewService(sources *Loader, payloads *Aggregator) *Service {
	return &Service{sources: sources, payloads: payloads}
}

// Close 释放 Service 所拥有的 payload 编排器。source snapshot 是每次 Search
// 的局部资源，已经在 Search 返回前关闭；长期存活的 provider/session 资源则
// 由 payloads 释放。
func (s *Service) Close() error {
	return s.payloads.Close()
}

func (s *Service) Search(ctx context.Context, request Request) (response Response, err error) {
	if ctx == nil {
		return Response{}, NewError(CodeInvalidRequest, "reverse search context is required", nil)
	}
	if err := s.payloads.preflight(ctx, request.Provider); err != nil {
		return Response{}, err
	}
	snapshot, err := s.sources.Load(ctx, request.Source)
	if err != nil {
		return Response{}, err
	}
	defer func() {
		if closeErr := snapshot.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()

	response, err = s.payloads.search(ctx, snapshot, request.Provider, request.PixivOnly)
	response.Input = Input{Kind: snapshot.Kind(), SHA256: snapshot.SHA256()}
	return response, err
}

var _ Searcher = (*Service)(nil)
var _ Closer = (*Service)(nil)
