// Package outputs defines FANBOX tool envelopes and safe resource projections.
package outputs

import (
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/toolrun"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Result 统一 MCP tool 的文本摘要；完整实体只存在于 structured output。
func Result(isError bool, message string) *mcp.CallToolResult {
	if message == "" {
		message = "OK"
	}
	return &mcp.CallToolResult{
		IsError: isError,
		Content: []mcp.Content{&mcp.TextContent{Text: message}},
	}
}

// ResourceOut 是资源引用的安全输出形态。
type ResourceOut struct {
	Ref                 string `json:"ref"`
	RequiresCredentials bool   `json:"requires_credentials,omitempty"`
}

// ResourceOutFrom 把 opaque Resource 转为安全输出。
func ResourceOutFrom(res sdk.Resource) *ResourceOut {
	if res.Ref.IsZero() {
		return nil
	}
	return &ResourceOut{Ref: res.Ref.String(), RequiresCredentials: res.RequiresCredentials}
}

// PostAssetOut 是帖子资产的安全输出形态。
type PostAssetOut struct {
	ID        string       `json:"id"`
	Kind      string       `json:"kind"`
	Name      string       `json:"name,omitempty"`
	Resource  *ResourceOut `json:"resource,omitempty"`
	Thumbnail *ResourceOut `json:"thumbnail,omitempty"`
}

// PostOut 是 FANBOX 帖子的安全输出形态。
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

// PostOutFrom 把 SDK post 转为安全输出。
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

// PostsOut 是帖子列表 tool 的输出 envelope。
type PostsOut struct {
	Posts      []PostOut             `json:"posts"`
	Pagination toolrun.PaginationOut `json:"pagination"`
}
