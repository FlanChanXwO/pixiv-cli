package pixiv

import (
	"fmt"

	recordpkg "github.com/FlanChanXwO/pixiv-cli/internal/record"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// recordResult 统一实体查询的 MCP 文本摘要。完整实体只存在于 structured
// records，Content 绝不复制 JSON，以便客户端始终只消费一个机器可读载荷。
func recordResult(records []recordpkg.Record, isError bool, message string) *mcp.CallToolResult {
	if message == "" {
		message = fmt.Sprintf("Retrieved %d records.", len(records))
	}
	return &mcp.CallToolResult{
		IsError: isError,
		Content: []mcp.Content{&mcp.TextContent{Text: message}},
	}
}

func recordErrorMessage(err error) string {
	return "Error: " + err.Error()
}

func recordsFromArtworks(items []pixiv.Artwork) ([]recordpkg.Record, error) {
	records := make([]recordpkg.Record, 0, len(items))
	for _, item := range items {
		record, err := recordpkg.RecordFromArtwork(item)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func recordsFromNovels(items []pixiv.Novel) ([]recordpkg.Record, error) {
	records := make([]recordpkg.Record, 0, len(items))
	for _, item := range items {
		record, err := recordpkg.RecordFromNovel(item)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func recordsFromUserPreviews(items []pixiv.UserPreview) ([]recordpkg.Record, error) {
	records := make([]recordpkg.Record, 0, len(items))
	for _, item := range items {
		record, err := recordpkg.RecordFromUserPreview(item)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}
