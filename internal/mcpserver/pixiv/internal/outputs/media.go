package outputs

import "github.com/modelcontextprotocol/go-sdk/mcp"

// ArtworkMedia indexes only bytes actually included in the MCP content array.
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

type MediaPage struct {
	Filename            string `json:"filename,omitempty"`
	PreviewContentIndex *int   `json:"preview_content_index,omitempty"`
	ArchiveQuality      string `json:"archive_quality,omitempty"`
	Page                int    `json:"page"`
	MIMEType            string `json:"mime_type"`
	Size                int    `json:"size"`
	ContentIndex        int    `json:"content_index"`
}

type MediaFailure struct {
	Page       int    `json:"page"`
	Error      string `json:"error"`
	HTTPStatus int    `json:"http_status,omitempty"`
}

func NewArtworkMedia(id int64) ArtworkMedia {
	return ArtworkMedia{IllustID: id, RequestedPages: []int{}, DeliveredPages: []int{}, Pages: []MediaPage{}, Failures: []MediaFailure{}}
}

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

func ProtectMediaInput(server *mcp.Server, name string) {
	protectInput(server, name, func() *mcp.CallToolResult {
		out := NewArtworkMedia(0)
		out.Error = "invalid_request"
		result, _, _ := MediaResult(out, nil)
		result.StructuredContent = out
		return result
	})
}
