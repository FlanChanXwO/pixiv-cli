package mcpserver

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
)

// pageLimitIn 以指针区分未传 limit（兼容旧的“单上游批次”）和显式 limit=0（读取全部）。
// Cursor 始终是 SDK 内部细节，不出现在 MCP tool 输入或输出中。
type pageLimitIn struct {
	Page  *int `json:"page,omitempty" jsonschema:"1-based logical page; requires a positive limit"`
	Limit *int `json:"limit,omitempty" jsonschema:"maximum items; 0 returns all items"`
}

type mcpListPlan struct {
	page     int
	limit    int
	oneBatch bool
	skip     int
}

type paginationOut struct {
	Page     int  `json:"page"`
	Limit    *int `json:"limit"`
	Returned int  `json:"returned"`
	HasMore  bool `json:"has_more"`
	NextPage *int `json:"next_page"`
}

func parseMCPListPlan(input pageLimitIn) (mcpListPlan, error) {
	if input.Page != nil && *input.Page <= 0 {
		return mcpListPlan{}, errors.New("page must be a positive integer")
	}
	if input.Limit != nil && *input.Limit < 0 {
		return mcpListPlan{}, errors.New("limit must be zero or a positive integer")
	}
	if input.Page != nil {
		if input.Limit == nil || *input.Limit <= 0 {
			return mcpListPlan{}, errors.New("page requires limit to be a positive integer")
		}
		if *input.Page-1 > math.MaxInt / *input.Limit {
			return mcpListPlan{}, errors.New("page and limit overflow the logical result offset")
		}
		return mcpListPlan{page: *input.Page, limit: *input.Limit, skip: (*input.Page - 1) * *input.Limit}, nil
	}
	if input.Limit == nil {
		return mcpListPlan{page: 1, limit: -1, oneBatch: true}, nil
	}
	return mcpListPlan{page: 1, limit: *input.Limit}, nil
}

func listPagination(plan mcpListPlan, limit *int, returned int, hasMore bool) paginationOut {
	out := paginationOut{Page: plan.page, Limit: limit, Returned: returned, HasMore: hasMore}
	if hasMore && limit != nil && *limit > 0 && plan.page < math.MaxInt {
		next := plan.page + 1
		out.NextPage = &next
	}
	return out
}

// collectPages 仅把 MCP 的兼容 sentinel 映射到 application 分页语义；成功空结果
// 仍保持 non-nil slice，失败时共享 collector 会丢弃部分结果。
func collectPages[T any](ctx context.Context, plan mcpListPlan, fetch func(context.Context, sdk.Cursor) ([]T, sdk.Cursor, error)) ([]T, bool, error) {
	limit := plan.limit
	if limit < 0 {
		limit = 0
	}
	items, result, err := application.CollectPages(ctx, application.PagePlan{
		Skip:     plan.skip,
		Limit:    limit,
		OneBatch: plan.oneBatch,
	}, fetch)
	if err != nil {
		return nil, false, err
	}
	return items, result.HasMore, nil
}

func (a *App) openSDKOperation(ctx context.Context) (client application.SDKClient, release func(), err error) {
	started := time.Now()
	defer func() { a.operationLog("open_sdk_operation", started, err != nil, err, 0, 0) }()
	if a.sdk.NewClient == nil {
		return nil, nil, errors.New("pixiv sdk is not configured")
	}
	if err = a.acquireSDKGate(ctx); err != nil {
		return nil, nil, err
	}
	client, err = a.sdk.OpenOperation(ctx, a.sdkRequest)
	if err != nil {
		a.releaseSDKGate()
		return nil, nil, err
	}
	return client, a.releaseSDKOperation, nil
}

// openSDKMutable 用于账号导入、选择和刷新。它持有与普通 operation 相同的 gate，
// 但不能先 Snapshot：ImportAccount 的输入 refresh token 尚未进入本地 store。
func (a *App) openSDKMutable(ctx context.Context) (client application.SDKClient, release func(), err error) {
	if a.sdk.NewClient == nil {
		return nil, nil, errors.New("pixiv sdk is not configured")
	}
	if err = a.acquireSDKGate(ctx); err != nil {
		return nil, nil, err
	}
	client, err = a.sdk.Client(a.sdkRequest)
	if err != nil {
		a.releaseSDKGate()
		return nil, nil, err
	}
	return client, a.releaseSDKGate, nil
}

func (a *App) currentSDKUser(ctx context.Context) (client application.SDKClient, userID int64, release func(), err error) {
	started := time.Now()
	defer func() { a.operationLog("current_sdk_user", started, err != nil, err, 0, userID) }()
	if a.sdk.NewClient == nil {
		return nil, 0, nil, errors.New("pixiv sdk is not configured")
	}
	if err = a.acquireSDKGate(ctx); err != nil {
		return nil, 0, nil, err
	}
	client, err = a.sdk.OpenOperation(ctx, a.sdkRequest)
	if err != nil {
		a.releaseSDKGate()
		return nil, 0, nil, err
	}
	userID, err = client.CurrentUserID(ctx)
	if err != nil {
		a.releaseSDKGate()
		return nil, 0, nil, err
	}
	return client, userID, a.releaseSDKOperation, nil
}

// operationLog 保持 MCP stdio 的 stdout 只属于 JSON-RPC。它不记录 tool 参数、
// 原始错误、认证材料或 URL；详细上游元数据由注入后的 public SDK 单独安全记录。
func (a *App) operationLog(operation string, started time.Time, failed bool, err error, illustID, userID int64) {
	if a == nil || a.logger == nil {
		return
	}
	result, code, backend, status := "success", "", "local", 0
	level := slog.LevelInfo
	var typed *sdk.Error
	if failed {
		result = "error"
		level = slog.LevelError
	}
	if err != nil {
		if errors.As(err, &typed) {
			code, backend, status = safeMCPErrorCode(typed.Code), safeMCPBackend(typed.Backend), typed.UpstreamStatus
			if typed.IllustID != 0 {
				illustID = typed.IllustID
			}
			if typed.UserID != 0 {
				userID = typed.UserID
			}
		}
	}
	attrs := []slog.Attr{slog.String("component", "mcp"), slog.String("operation", operation), slog.String("backend", backend), slog.Duration("duration", time.Since(started)), slog.String("result", result), slog.String("error_code", code), slog.Int("status", status)}
	if illustID != 0 {
		attrs = append(attrs, slog.Int64("illust_id", illustID))
	}
	if userID != 0 {
		attrs = append(attrs, slog.Int64("user_id", userID))
	}
	a.logger.LogAttrs(nil, level, "pixiv operation", attrs...)
}

// safeMCPErrorCode 只允许公开 SDK 定义的稳定枚举进入 stderr；Error 的字段
// 对外可写，未知字符串不能被当作任意日志载荷。
func safeMCPErrorCode(code sdk.ErrorCode) string {
	switch code {
	case sdk.CodeInvalidArgument,
		sdk.CodeArtworkUnavailable,
		sdk.CodeUnauthorized,
		sdk.CodeForbidden,
		sdk.CodeUnsupported,
		sdk.CodeRateLimited,
		sdk.CodeUpstreamError,
		sdk.CodeUpstreamUnavailable,
		sdk.CodeMalformedUpstreamResponse:
		return string(code)
	default:
		return ""
	}
}

// safeMCPBackend 与无 backend 的既有日志语义一致，把未知值归为 local；它不
// 解析或清洗任意字符串，避免 URL、token 或其他调用方载荷进入事件。
func safeMCPBackend(backend sdk.Backend) string {
	switch backend {
	case sdk.BackendAppAPI,
		sdk.BackendWebAPI,
		sdk.BackendOAuth,
		sdk.BackendResource:
		return string(backend)
	default:
		return "local"
	}
}

func (a *App) releaseSDKOperation() {
	a.releaseSDKGate()
}

func (a *App) acquireSDKGate(ctx context.Context) error {
	if a.sdkGate == nil {
		a.sdkGate = make(chan struct{}, 1)
	}
	select {
	case a.sdkGate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *App) releaseSDKGate() { <-a.sdkGate }

func resolveSDKUser(ctx context.Context, app *App, userID int64) (application.SDKClient, int64, func(), error) {
	if userID != 0 {
		client, release, err := app.openSDKOperation(ctx)
		return client, userID, release, err
	}
	return app.currentSDKUser(ctx)
}
