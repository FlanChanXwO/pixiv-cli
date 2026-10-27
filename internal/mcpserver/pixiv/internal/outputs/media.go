package outputs

import (
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/toolrun"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ArtworkMedia 描述一次作品媒体请求的结果：每页的索引、大小与失败项。Complete 为 false
// 时 Error 或 Failures 说明缺了哪一页。
type ArtworkMedia struct {
	IllustID       int64          `json:"illust_id"`
	Title          string         `json:"title"`
	TotalPages     int            `json:"total_pages"`
	RequestedPages []int          `json:"requested_pages"`
	DeliveredPages []int          `json:"delivered_pages"`
	Pages          []MediaPage    `json:"pages"`
	Failures       []MediaFailure `json:"failures"`
	Complete       bool           `json:"complete"`
	Error          string         `json:"error,omitempty"`
}

// MediaPage 记录一页媒体的交付信息；Filename 与 ArchiveQuality 只在有对应产物时填写。
type MediaPage struct {
	Filename            string `json:"filename,omitempty"`
	PreviewContentIndex *int   `json:"preview_content_index,omitempty"`
	ArchiveQuality      string `json:"archive_quality,omitempty"`
	Page                int    `json:"page"`
	MIMEType            string `json:"mime_type"`
	Size                int    `json:"size"`
	ContentIndex        int    `json:"content_index"`
}

// MediaFailure 记录单页失败；作品级失败放在 ArtworkMedia.Error，不伪造页号。
type MediaFailure struct {
	Page       int    `json:"page"`
	Error      string `json:"error"`
	HTTPStatus int    `json:"http_status,omitempty"`
}

// NewArtworkMedia 返回所有切片均为空数组的初始结果，便于稳定序列化。
func NewArtworkMedia(id int64) ArtworkMedia {
	return ArtworkMedia{IllustID: id, RequestedPages: []int{}, DeliveredPages: []int{}, Pages: []MediaPage{}, Failures: []MediaFailure{}}
}

// MediaResult 用交付内容生成结果；没有内容时退回 out.Error，仍缺省为 media_incomplete。
func MediaResult(out ArtworkMedia, content []mcp.Content) (*mcp.CallToolResult, ArtworkMedia, error) {
	if len(content) == 0 {
		text := out.Error
		if text == "" {
			text = "media_incomplete"
		}
		content = []mcp.Content{&mcp.TextContent{Text: text}}
	}
	return &mcp.CallToolResult{IsError: !out.Complete, Content: content}, out, nil
}

// ProtectMediaInput 把参数校验失败替换成媒体工具声明的结构化 envelope。
func ProtectMediaInput(server *mcp.Server, name string) {
	toolrun.ProtectInput(server, name, func() *mcp.CallToolResult {
		out := NewArtworkMedia(0)
		out.Error = "invalid_request"
		result, _, _ := MediaResult(out, nil)
		result.StructuredContent = out
		return result
	})
}
