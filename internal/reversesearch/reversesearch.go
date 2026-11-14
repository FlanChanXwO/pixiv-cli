// Package reversesearch 读取反向搜图来源，并合并 SauceNAO 与 ascii2d SDK 的结果。
//
// 包的组成：[New] 接收调用方读取的配置与连接，[Service.Search] 返回稳定的 [Response]；
// [Loader] 与 [Snapshot] 管理私有载荷，[Error] 保持 CLI/MCP 的错误分类。
package reversesearch

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/FlanChanXwO/pixiv-cli/sdk/ascii2d"
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

// Options 固定调用方提供的凭据、标准 HTTP 连接及 ascii2d 独立网络配置。
// HTTPClient 由调用方创建，供来源读取与 SauceNAO 使用；Close 释放其空闲连接。
type Options struct {
	HTTPClient           *http.Client
	SauceNAOKey          string
	ASCII2DProxy         string
	UserAgent            string
	FlareSolverrURL      string
	FlareSolverrProxyURL string
}

// Service 将同一份来源交给两个独立 SDK，并按 Pixiv 身份合并结果。
type Service struct {
	sources  *Loader
	options  Options
	mu       sync.Mutex
	closeErr error
}

// New 检查调用方传入的连接配置，不读取本地配置、账号或发起请求。
func New(options Options) (*Service, error) {
	// 配置检查保持在来源读取之前，避免无效连接设置导致先读取或下载图片。
	if strings.ContainsAny(options.UserAgent, "\r\n\x00") {
		return nil, NewError(CodeInvalidRequest, "ascii2d user-agent contains invalid header characters", nil)
	}
	if strings.TrimSpace(options.ASCII2DProxy) != "" {
		proxy, err := url.Parse(options.ASCII2DProxy)
		if err != nil || proxy.Hostname() == "" ||
			(proxy.Scheme != "http" && proxy.Scheme != "https" && proxy.Scheme != "socks5" && proxy.Scheme != "socks5h") ||
			(proxy.Path != "" && proxy.Path != "/") || proxy.RawQuery != "" || proxy.Fragment != "" || proxy.ForceQuery {
			return nil, NewError(CodeInvalidRequest, "ascii2d proxy URL is invalid", err)
		}
	}
	if options.FlareSolverrURL != "" {
		options.FlareSolverrURL = strings.TrimSpace(options.FlareSolverrURL)
		solver, err := url.Parse(options.FlareSolverrURL)
		if err != nil || solver.Host == "" || solver.User != nil || solver.RawQuery != "" || solver.Fragment != "" || solver.ForceQuery ||
			(solver.Scheme != "http" && solver.Scheme != "https") {
			return nil, NewError(CodeSolverUnavailable, "ascii2d challenge solver is unavailable", err)
		}
	}
	return &Service{sources: NewSourceLoader(options.HTTPClient), options: options}, nil
}

// Close 释放标准 HTTP 连接，并报告查询期间 SDK 返回的资源关闭错误；重复调用返回相同错误。
func (s *Service) Close() error {
	s.options.HTTPClient.CloseIdleConnections()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeErr
}

func (s *Service) retainCloseError(err error) {
	if err == nil {
		return
	}
	// SDK 在查询返回前清理资源；错误仍由 CLI/MCP 既有的关闭阶段报告。
	s.mu.Lock()
	s.closeErr = errors.Join(s.closeErr, err)
	s.mu.Unlock()
}

func (s *Service) preflight(ctx context.Context, provider Provider) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch provider {
	case ProviderSauceNAO:
		return s.sauceNAOCredential()
	case ProviderASCII2DColor, ProviderASCII2DBOVW, ProviderAll:
		return nil
	default:
		return NewError(CodeInvalidRequest, "reverse search provider is invalid", nil)
	}
}

func (s *Service) sauceNAOCredential() error {
	if strings.TrimSpace(s.options.SauceNAOKey) == "" {
		return NewError(CodeMissingCredential, "SauceNAO API key is required", nil)
	}
	return nil
}

func (s *Service) ascii2dOptions(image []byte, mode ascii2d.Mode) ascii2d.Options {
	return ascii2d.Options{Image: image, Mode: mode, ProxyURL: s.options.ASCII2DProxy,
		UserAgent: s.options.UserAgent, FlareSolverrURL: s.options.FlareSolverrURL,
		FlareSolverrProxyURL: s.options.FlareSolverrProxyURL}
}

// Search 校验 provider，载入一次不可变快照，再把快照交给编排器查询。快照在返回前
// 删除，其关闭错误与查询错误一起返回；nil context 返回 invalid_request。
func (s *Service) Search(ctx context.Context, request Request) (response Response, err error) {
	if ctx == nil {
		return Response{}, NewError(CodeInvalidRequest, "reverse search context is required", nil)
	}
	if err := s.preflight(ctx, request.Provider); err != nil {
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

	if err := ctx.Err(); err != nil {
		return Response{Input: Input{Kind: snapshot.Kind(), SHA256: snapshot.SHA256()}}, err
	}

	// ascii2d 的既有上限在读入内存前执行；SauceNAO 与 all 的输入范围保持不变。
	if (request.Provider == ProviderASCII2DColor || request.Provider == ProviderASCII2DBOVW) && snapshot.Size() > ascii2d.MaxImageBytes {
		response, err = responseFromFailure(request.Provider, NewError(CodeInvalidSource, "ascii2d image exceeds the 10 MB limit", nil))
		response.Input = Input{Kind: snapshot.Kind(), SHA256: snapshot.SHA256()}
		return response, err
	}

	reader, err := snapshot.Open(ctx)
	if err != nil {
		return Response{}, err
	}
	image, readErr := io.ReadAll(reader)
	if err := errors.Join(readErr, reader.Close()); err != nil {
		return Response{}, NewError(CodeSnapshotFailed, "could not read image snapshot", err)
	}
	response, err = s.search(ctx, image, request.Provider, request.PixivOnly)
	response.Input = Input{Kind: snapshot.Kind(), SHA256: snapshot.SHA256()}
	return response, err
}

var _ Searcher = (*Service)(nil)
var _ Closer = (*Service)(nil)
