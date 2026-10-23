package mcpserver

import (
	"context"
	"encoding/json"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func observeMediaResult(ctx context.Context, request mcp.Request, result mcp.Result) mcp.Result {
	call, ok := request.(*mcp.CallToolRequest)
	if result == nil || !ok || (call.Params.Name != "pixiv_artwork_media" && call.Params.Name != "pixiv_gallery_animation") {
		return result
	}
	// HTTP 与统一协议 owner 可同时安装观测；每个结果只包装一次。
	if _, wrapped := result.(*observedMediaResult); wrapped {
		return result
	}
	if _, scoped := logging.ScopeFromContext(ctx); !scoped {
		return result
	}
	return &observedMediaResult{Result: result, ctx: ctx}
}

// 在 SDK 实际序列化边界计时，不提前编码、不记录结果正文。
type observedMediaResult struct {
	mcp.Result
	ctx context.Context
}

func (r *observedMediaResult) MarshalJSON() ([]byte, error) {
	started := time.Now()
	data, err := json.Marshal(r.Result)
	event := logging.Event{Module: logging.ModulePixivMCP, Kind: logging.EventCompleted, Operation: "mcp_response_serialization", Duration: time.Since(started), Count: len(data)}
	if err != nil {
		event.Kind = logging.EventFailed
		event.Reason = logging.ReasonToolFailed
	}
	logging.Emit(r.ctx, event)
	return data, err
}
