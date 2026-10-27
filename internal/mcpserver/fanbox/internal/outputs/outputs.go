// Package outputs 定义 FANBOX 工具的文本摘要、结构化 envelope 与安全的资源投影。
//
// 包的组成：[Result] 生成工具的文本摘要；[ResourceOut]、[PostAssetOut] 与 [PostOut]
// 是只暴露不透明 ref、不带下载地址的输出形态，[ResourceOutFrom] 与 [PostOutFrom]
// 负责从 SDK 模型转换。
package outputs

import (
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/toolrun"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Result 生成 MCP tool 的文本摘要；完整实体只放在 structured output 里，message 为空时
// 输出 OK。
func Result(isError bool, message string) *mcp.CallToolResult {
	if message == "" {
		message = "OK"
	}
	return &mcp.CallToolResult{
		IsError: isError,
		Content: []mcp.Content{&mcp.TextContent{Text: message}},
	}
}

// ResourceOut 只暴露资源的 opaque ref 与是否需要凭据。
type ResourceOut struct {
	Ref                 string `json:"ref"`
	RequiresCredentials bool   `json:"requires_credentials,omitempty"`
}

// ResourceOutFrom 只保留资源的 opaque ref 与凭据要求；零值 ref 返回 nil。
func ResourceOutFrom(res sdk.Resource) *ResourceOut {
	if res.Ref.IsZero() {
		return nil
	}
	return &ResourceOut{Ref: res.Ref.String(), RequiresCredentials: res.RequiresCredentials}
}

// PostAssetOut 表示帖子中的一个资产；Resource 与 Thumbnail 只暴露不透明 ref。
type PostAssetOut struct {
	ID        string       `json:"id"`
	Kind      string       `json:"kind"`
	Name      string       `json:"name,omitempty"`
	Resource  *ResourceOut `json:"resource,omitempty"`
	Thumbnail *ResourceOut `json:"thumbnail,omitempty"`
}

// PostOut 承载不带原始资源地址的 FANBOX 帖子。PublishedAt 为 RFC 3339 UTC 文本，
// 时间未知时为空串。
type PostOut struct {
	ID            string         `json:"id"`
	Title         string         `json:"title"`
	PublishedAt   string         `json:"published_at"`
	CreatorID     string         `json:"creator_id"`
	FeeRequired   int            `json:"fee_required,omitempty"`
	IsRestricted  bool           `json:"is_restricted"`
	IsPinned      bool           `json:"is_pinned,omitempty"`
	RestrictedFor int            `json:"restricted_for,omitempty"`
	CommentCount  int            `json:"comment_count,omitempty"`
	Assets        []PostAssetOut `json:"assets"`
}

// PostOutFrom 丢弃 Body 中的下载地址，只保留资源 ref；Assets 总是非 nil。
func PostOutFrom(post fanbox.Post) PostOut {
	published := ""
	if !post.PublishedAt.IsZero() {
		published = post.PublishedAt.UTC().Format(time.RFC3339)
	}
	out := PostOut{
		ID:            post.ID,
		Title:         post.Title,
		PublishedAt:   published,
		CreatorID:     post.CreatorID,
		FeeRequired:   post.FeeRequired,
		IsRestricted:  post.IsRestricted,
		IsPinned:      post.IsPinned,
		RestrictedFor: post.RestrictedFor,
		CommentCount:  post.CommentCount,
		Assets:        []PostAssetOut{},
	}
	if post.Body == nil {
		return out
	}
	for _, asset := range post.Body.Assets {
		out.Assets = append(out.Assets, PostAssetOut{
			ID:        asset.ID,
			Kind:      string(asset.Kind),
			Name:      asset.Name,
			Resource:  ResourceOutFrom(asset.Resource),
			Thumbnail: ResourceOutFrom(asset.Thumbnail.Resource),
		})
	}
	return out
}

// PostsOut 承载帖子列表工具的输出。
type PostsOut struct {
	Posts      []PostOut             `json:"posts"`
	Pagination toolrun.PaginationOut `json:"pagination"`
}
