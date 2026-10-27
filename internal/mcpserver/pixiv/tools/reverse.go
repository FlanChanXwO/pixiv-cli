package tools

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/toolhost"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/toolrun"
	record "github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/internal/reversesearch"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerReverseSearch 注册 pixiv_reverse_search。source 只在当前请求中交给反向搜图服务，
// 输出只暴露来源类别与摘要。provider 可由请求覆盖，缺省取启动快照；pixiv-only、代理与
// 凭据只来自启动快照。
func (t pixivTools) registerReverseSearch(server *mcp.Server) {
	// SDK 的 schema 与类型错误会带出输入值，所以只替换尚未进入 typed handler 的失败；
	// handler 总是返回完整 envelope，provider 的部分结果与错误原样保留。
	toolrun.ProtectInput(server, "pixiv_reverse_search", func() *mcp.CallToolResult {
		out := emptyReverseSearchOutput()
		safe := reverseSearchError(reversesearch.CodeInvalidRequest)
		safe.StructuredContent = out
		return safe
	})
	toolhost.AddTool(t.app, server, &mcp.Tool{
		Meta: mcp.Meta{"openai/fileParams": []string{"image"}},
		Name: "pixiv_reverse_search", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)},
		Description:  "Upload a server-side image source or a host-provided image to SauceNAO or ascii2d and return Pixiv matches. Provide exactly one of source or image.",
		InputSchema:  reverseSearchInputSchema(),
		OutputSchema: reverseSearchOutputSchema(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input reverseSearchInput) (*mcp.CallToolResult, reverseSearchOutput, error) {
		return t.reverseSearchHandler(ctx, input)
	})
}

type reverseSearchInput struct {
	Source   *string             `json:"source,omitempty"`
	Image    *reverseSearchImage `json:"image,omitempty"`
	Provider string              `json:"provider,omitempty"`
}

// reverseSearchImage 只把下载地址交给 provider；MIME 类型与文件名不参与搜图，也不回显。
type reverseSearchImage struct {
	DownloadURL string `json:"download_url"`
	FileID      string `json:"file_id"`
	MIMEType    string `json:"mime_type,omitempty"`
	FileName    string `json:"file_name,omitempty"`
}

// reverseSearchOutput 是完整的 MCP wire envelope，structured content 里不出现原始 source、
// 临时路径、请求头、cookie 或 API key。
type reverseSearchOutput struct {
	Input          reversesearch.Input             `json:"input"`
	Providers      []reversesearch.ProviderSummary `json:"providers"`
	Results        []reversesearch.Result          `json:"results"`
	Records        []record.Record                 `json:"records"`
	ProviderErrors []reversesearch.ProviderError   `json:"provider_errors"`
	Partial        bool                            `json:"partial"`
}

func reverseSearchInputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"oneOf":                []map[string]any{{"required": []string{"source"}}, {"required": []string{"image"}}},
		"properties": map[string]any{
			"source": map[string]any{
				"type":        "string",
				"description": "Readable regular file path, local file:// URI, or HTTP(S) image URL on the MCP server, not the connector device. The source is fetched/uploaded and is never returned.",
			},
			"image": map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []string{"download_url", "file_id"},
				"properties": map[string]any{
					"download_url": map[string]any{"type": "string", "description": "HTTP(S) download URL supplied by the host."},
					"file_id":      map[string]any{"type": "string", "description": "Host file identifier; not a download address."},
					"mime_type":    map[string]any{"type": "string"},
					"file_name":    map[string]any{"type": "string"},
				},
			},
			"provider": map[string]any{
				"type":        "string",
				"enum":        []string{string(reversesearch.ProviderSauceNAO), string(reversesearch.ProviderASCII2DColor), string(reversesearch.ProviderASCII2DBOVW), string(reversesearch.ProviderAll)},
				"description": "Optional provider override; omit to use the server startup configuration.",
			},
		},
	}
}

func reverseSearchOutputSchema() map[string]any {
	recordIdentity := map[string]any{
		"type":                 "object",
		"required":             []string{"type", "id", "url"},
		"properties":           map[string]any{"type": map[string]any{"type": "string"}, "id": map[string]any{"type": "string"}, "url": map[string]any{"type": "string"}},
		"additionalProperties": map[string]any{},
	}
	pixivIdentity := map[string]any{
		"type":                 "object",
		"required":             []string{"type", "id"},
		"properties":           map[string]any{"type": map[string]any{"type": "string"}, "id": map[string]any{"type": "integer"}},
		"additionalProperties": false,
	}
	quota := map[string]any{
		"type":                 "object",
		"required":             []string{"short_remaining", "long_remaining", "short_limit", "long_limit"},
		"properties":           map[string]any{"short_remaining": map[string]any{"type": "integer"}, "long_remaining": map[string]any{"type": "integer"}, "short_limit": map[string]any{"type": "integer"}, "long_limit": map[string]any{"type": "integer"}},
		"additionalProperties": false,
	}
	providerSummary := map[string]any{
		"type":                 "object",
		"required":             []string{"name", "status", "result_count"},
		"properties":           map[string]any{"name": map[string]any{"type": "string"}, "status": map[string]any{"type": "string"}, "result_count": map[string]any{"type": "integer"}, "quota": quota},
		"additionalProperties": false,
	}
	evidence := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"provider":      map[string]any{"type": "string"},
			"rank":          map[string]any{"type": "integer"},
			"similarity":    map[string]any{"type": "number"},
			"index_id":      map[string]any{"type": "integer"},
			"index_name":    map[string]any{"type": "string"},
			"title":         map[string]any{"type": "string"},
			"author":        map[string]any{"type": "string"},
			"external_urls": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
		"required":             []string{"provider", "rank", "similarity", "index_id", "index_name"},
		"additionalProperties": false,
	}
	result := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pixiv":    pixivIdentity,
			"title":    map[string]any{"type": "string"},
			"author":   map[string]any{"type": "string"},
			"evidence": map[string]any{"type": "array", "items": evidence},
		},
		"required":             []string{"evidence"},
		"additionalProperties": false,
	}
	providerError := map[string]any{
		"type":                 "object",
		"required":             []string{"provider", "code", "message"},
		"properties":           map[string]any{"provider": map[string]any{"type": "string"}, "code": map[string]any{"type": "string"}, "message": map[string]any{"type": "string"}},
		"additionalProperties": false,
	}
	input := map[string]any{
		"type":                 "object",
		"required":             []string{"kind", "sha256"},
		"properties":           map[string]any{"kind": map[string]any{"type": "string"}, "sha256": map[string]any{"type": "string"}},
		"additionalProperties": false,
	}
	return map[string]any{
		"type":     "object",
		"required": []string{"input", "providers", "results", "records", "provider_errors", "partial"},
		"properties": map[string]any{
			"input": input,
			"providers": map[string]any{
				"type":  "array",
				"items": providerSummary,
			},
			"results": map[string]any{
				"type":  "array",
				"items": result,
			},
			"records": map[string]any{
				"type":  "array",
				"items": recordIdentity,
			},
			"provider_errors": map[string]any{
				"type":  "array",
				"items": providerError,
			},
			"partial": map[string]any{"type": "boolean"},
		},
		"additionalProperties": false,
	}
}

func (t pixivTools) reverseSearchHandler(ctx context.Context, input reverseSearchInput) (*mcp.CallToolResult, reverseSearchOutput, error) {
	out := emptyReverseSearchOutput()
	if (input.Source == nil) == (input.Image == nil) {
		return reverseSearchError(reversesearch.CodeInvalidRequest), out, nil
	}
	var source string
	if input.Source != nil {
		source = *input.Source
		if strings.HasPrefix(strings.ToLower(source), "file://") {
			parsed, err := url.Parse(source)
			// file URI 只接受 server 本机绝对路径；带 authority、查询或片段时直接拒绝。
			if err != nil || parsed.User != nil || (parsed.Host != "" && !strings.EqualFold(parsed.Host, "localhost")) || parsed.RawQuery != "" || parsed.ForceQuery || strings.Contains(source, "#") || strings.ContainsRune(parsed.Path, 0) {
				return reverseSearchError(reversesearch.CodeInvalidSource), out, nil
			}
			source = filepath.FromSlash(parsed.Path)
			// Windows 的 file:///C:/... 多出一个前导分隔符，需要去掉；其他平台保持原样。
			if len(source) > 0 && filepath.VolumeName(source[1:]) != "" {
				source = source[1:]
			}
			if !filepath.IsAbs(source) || strings.HasPrefix(filepath.ToSlash(source), "//") {
				return reverseSearchError(reversesearch.CodeInvalidSource), out, nil
			}
		}
	} else {
		source = input.Image.DownloadURL
		parsed, err := url.Parse(source)
		if strings.TrimSpace(input.Image.FileID) == "" || err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return reverseSearchError(reversesearch.CodeInvalidRequest), out, nil
		}
	}
	if strings.TrimSpace(source) == "" {
		return reverseSearchError(reversesearch.CodeInvalidRequest), out, nil
	}
	ports := t.app.ReverseSearchPorts()
	if ports.Searcher == nil {
		return reverseSearchError(reversesearch.CodeProviderNotConfigured), out, nil
	}
	provider := ports.Provider
	if input.Provider != "" {
		provider = reversesearch.Provider(input.Provider)
	}
	if provider == "" {
		provider = reversesearch.ProviderSauceNAO
	}
	response, err := ports.Searcher.Search(ctx, reversesearch.Request{
		Source: source, Provider: provider, PixivOnly: ports.PixivOnly,
	})
	out = outputFromResponse(response)
	// Search 返回错误时仍可能已有完成的 Pixiv 结果（例如 Snapshot.Close 失败），先填
	// records，让 isError=true 的输出保留完整 envelope。
	records, recordsErr := reverseSearchRecords(response.Results)
	if recordsErr == nil {
		out.Records = records
	}
	if err != nil {
		code := safeErrorCode(ctx, err)
		result := reverseSearchError(code)
		if input.Image != nil && (code == reversesearch.CodeSourceHTTPStatus || code == reversesearch.CodeSourceReadFailed) {
			result.Content = append(result.Content, &mcp.TextContent{Text: "The host image could not be downloaded. Reattach the image and try again."})
		}
		return result, out, nil
	}
	if recordsErr != nil {
		return reverseSearchError(reversesearch.CodeUnknown), out, nil
	}
	return reverseSearchResult(out), out, nil
}

func emptyReverseSearchOutput() reverseSearchOutput {
	return reverseSearchOutput{
		Providers:      []reversesearch.ProviderSummary{},
		Results:        []reversesearch.Result{},
		Records:        []record.Record{},
		ProviderErrors: []reversesearch.ProviderError{},
	}
}

func outputFromResponse(response reversesearch.Response) reverseSearchOutput {
	out := emptyReverseSearchOutput()
	out.Input = response.Input
	if response.Providers != nil {
		out.Providers = response.Providers
	}
	if response.Results != nil {
		out.Results = make([]reversesearch.Result, 0, len(response.Results))
		for _, result := range response.Results {
			if result.Evidence == nil {
				result.Evidence = []reversesearch.Evidence{}
			}
			out.Results = append(out.Results, result)
		}
	}
	if response.ProviderErrors != nil {
		out.ProviderErrors = response.ProviderErrors
	}
	out.Partial = response.Partial
	return out
}

func reverseSearchRecords(results []reversesearch.Result) ([]record.Record, error) {
	items := make([]record.Record, 0, len(results))
	for _, result := range results {
		if result.Pixiv == nil {
			continue
		}
		var recordType string
		switch result.Pixiv.Type {
		case reversesearch.PixivRefArtwork:
			recordType = kindArtwork
		case reversesearch.PixivRefUser:
			recordType = kindUser
		default:
			return nil, errors.New("invalid reverse search identity")
		}
		identity, err := record.NewIdentityRecord(result.Pixiv.ID, recordType)
		if err != nil {
			return nil, err
		}
		items = append(items, identity)
	}
	return items, nil
}

func reverseSearchResult(out reverseSearchOutput) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Retrieved %d reverse-search results.", len(out.Results))}},
	}
}

func reverseSearchError(code reversesearch.ErrorCode) *mcp.CallToolResult {
	message := "Error: reverse search failed"
	if code != "" && code != reversesearch.CodeUnknown {
		message += " (" + string(code) + ")"
	}
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: message}},
	}
}

func safeErrorCode(ctx context.Context, err error) reversesearch.ErrorCode {
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return reversesearch.CodeUnknown
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return reversesearch.CodeUnknown
	}
	return reversesearch.CodeOf(err)
}
