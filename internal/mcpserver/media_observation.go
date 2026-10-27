package mcpserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func observeMediaResult(lifecycleCtx context.Context, request mcp.Request, result mcp.Result) mcp.Result {
	call, ok := request.(*mcp.CallToolRequest)
	if result == nil || !ok || (call.Params.Name != "pixiv_artwork_media" && call.Params.Name != "pixiv_gallery_animation") {
		return result
	}
	// HTTP 与统一协议 owner 可同时安装观测；每个结果只包装一次。
	if _, wrapped := result.(*observedMediaResult); wrapped {
		return result
	}
	if !logging.HasLogger(lifecycleCtx) {
		return result
	}
	return &observedMediaResult{Result: result, logger: logging.From(lifecycleCtx)}
}

// observedMediaResult 在 SDK 实际序列化时计时，不提前编码，也不记录结果正文。
type observedMediaResult struct {
	mcp.Result
	logger *slog.Logger
}

// MarshalJSON 记录序列化耗时与字节数；失败时只记录安全原因，不记录结果正文。
func (r *observedMediaResult) MarshalJSON() ([]byte, error) {
	started := time.Now()
	data, err := json.Marshal(r.Result)
	attrs := []slog.Attr{slog.String(logging.AttrStage, "mcp_response_serialization"), slog.Duration(logging.AttrDuration, time.Since(started)), slog.Int(logging.AttrCount, len(data))}
	if err != nil {
		r.logger.LogAttrs(context.Background(), slog.LevelWarn, "MCP response serialization failed", append(attrs, slog.String(logging.AttrReason, "tool failed"))...)
		return data, err
	}
	r.logger.LogAttrs(context.Background(), slog.LevelDebug, "MCP response serialized", attrs...)
	return data, err
}
