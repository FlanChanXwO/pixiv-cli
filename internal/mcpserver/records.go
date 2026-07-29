package mcpserver

import (
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// recordResult 统一实体查询的 MCP 文本摘要。完整实体只存在于 structured
// records，Content 绝不复制 JSON，以便客户端始终只消费一个机器可读载荷。
func recordResult(records []application.Record, isError bool, message string) *mcp.CallToolResult {
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

func recordsFromIllusts(items []sdk.Illust) ([]application.Record, error) {
	records := make([]application.Record, 0, len(items))
	for _, item := range items {
		record, err := application.RecordFromIllust(item)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func recordsFromNovels(items []sdk.Novel) ([]application.Record, error) {
	records := make([]application.Record, 0, len(items))
	for _, item := range items {
		record, err := application.RecordFromNovel(item)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func recordsFromUserPreviews(items []sdk.UserPreview) ([]application.Record, error) {
	records := make([]application.Record, 0, len(items))
	for _, item := range items {
		record, err := application.RecordFromUserPreview(item)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func recordsFromRecommendedUserPreviews(items []sdk.RecommendedUserPreview) ([]application.Record, error) {
	records := make([]application.Record, 0, len(items))
	for _, item := range items {
		record, err := application.RecordFromRecommendedUserPreview(item)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}
