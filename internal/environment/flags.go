package environment

import (
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	"github.com/spf13/cobra"
)

// BindNDJSONFlag 注册列表命令自己的记录流开关。
func BindNDJSONFlag(cmd *cobra.Command, value *bool) {
	cmd.Flags().BoolVar(value, "ndjson", false, "print one Pixiv entity record as JSON per line")
}

// BindListFlags 给列表命令注册逻辑分页 flag；未给 --limit 时只读一批。
func BindListFlags(cmd *cobra.Command, limit, page *int) {
	flags := cmd.Flags()
	flags.SetOutput(cmd.ErrOrStderr())
	flags.IntVarP(limit, "limit", "l", 0, "maximum results; omitted returns one upstream batch; 0 returns all results")
	flags.IntVarP(page, "page", "p", 0, "1-based logical page (requires --limit > 0)")
}

// BindFanboxListFlags 给 FANBOX 列表命令注册输出格式和逻辑分页 flag。
// FANBOX 的 --limit/--page 没有短选项，帮助文本与既有输出保持一致。
func BindFanboxListFlags(cmd *cobra.Command, jsonOut, ndjson *bool, limit, page *int) {
	flags := cmd.Flags()
	flags.BoolVar(jsonOut, "json", false, "print JSON")
	flags.BoolVar(ndjson, "ndjson", false, "print one item as JSON per line")
	flags.SetOutput(cmd.ErrOrStderr())
	flags.IntVar(limit, "limit", 0, "maximum results; omitted returns one upstream batch; 0 returns all results")
	flags.IntVar(page, "page", 0, "1-based logical page (requires --limit > 0)")
}

// BindFanboxSingleFlags 只给单实体命令注册 --json/--ndjson，避免接受会被静默忽略的 --limit/--page。
func BindFanboxSingleFlags(cmd *cobra.Command, jsonOut, ndjson *bool) {
	flags := cmd.Flags()
	flags.BoolVar(jsonOut, "json", false, "print JSON")
	flags.BoolVar(ndjson, "ndjson", false, "print one item as JSON per line")
}

// ParsePlan 校验 --limit/--page 并生成逻辑分页计划；未显式给 --limit 时只读一批。
func ParsePlan(cmd *cobra.Command, limit, page int) (pagination.PagePlan, error) {
	return pagination.ParsePlan(limit, page, cmd.Flags().Changed("limit"), cmd.Flags().Changed("page"))
}

// ListJSONOut 解析文档输出模式；同时给 --ndjson 和 --json 时按 usage 错误拒绝。
func (e Env) ListJSONOut(cmd *cobra.Command, jsonOut, ndjson bool) (bool, error) {
	if ndjson && cmd.Flags().Changed("json") {
		return false, e.WrapUsage(errors.New("--ndjson cannot be used with --json"))
	}
	return !ndjson && jsonOut, nil
}

// Request 从共用代理 flag 构造一次 Pixiv 执行请求。
func (e Env) Request(cmd *cobra.Command, opts ProxyOptions) (session.ProxyRequest, error) {
	proxy, err := ProxyOverride(cmd, opts)
	if err != nil {
		return session.ProxyRequest{}, err
	}
	return session.ProxyRequest{HTTPSProxyOverride: proxy}, nil
}
