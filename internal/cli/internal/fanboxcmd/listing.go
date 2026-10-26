// FANBOX 列表命令的 flag、输出模式与分页执行；逻辑分页与 JSON 暂存复用 command。

package fanboxcmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/spf13/cobra"
)

// Options 是 FANBOX 列表命令共享的输出与逻辑分页参数。FANBOX 客户端不消费 pixiv
// 代理配置，因此这里不绑定 --proxy/--no-proxy，避免接受却被静默忽略的 flag。
type Options struct {
	JSON   bool
	NDJSON bool
	Limit  int
	Page   int
}

// BindListFlags adds output and logical pagination flags to a list command.
func BindListFlags(cmd *cobra.Command, opts *Options) {
	flags := cmd.Flags()
	flags.BoolVar(&opts.JSON, "json", false, "print JSON")
	flags.BoolVar(&opts.NDJSON, "ndjson", false, "print one item as JSON per line")
	flags.SetOutput(cmd.ErrOrStderr())
	flags.IntVar(&opts.Limit, "limit", 0, "maximum results; omitted returns one upstream batch; 0 returns all results")
	flags.IntVar(&opts.Page, "page", 0, "1-based logical page (requires --limit > 0)")
}

// BindSingleFlags only exposes --json/--ndjson for single-entity commands so
// they never accept --limit/--page values that would be silently ignored.
func BindSingleFlags(cmd *cobra.Command, opts *Options) {
	flags := cmd.Flags()
	flags.BoolVar(&opts.JSON, "json", false, "print JSON")
	flags.BoolVar(&opts.NDJSON, "ndjson", false, "print one item as JSON per line")
}

// ParsePlan validates a command's logical pagination settings.
func ParsePlan(cmd *cobra.Command, opts Options) (command.Plan, error) {
	return command.ParsePlan(cmd, opts.Limit, opts.Page)
}

// JSONOut resolves the document output mode and rejects the --ndjson/--json
// combination through the command's usage-error wrapper.
func JSONOut(cmd *cobra.Command, opts Options, usageError func(error) error) (bool, error) {
	if opts.NDJSON && cmd.Flags().Changed("json") {
		return false, usageError(errors.New("--ndjson cannot be used with --json"))
	}
	return !opts.NDJSON && opts.JSON, nil
}

// Traverse 跟随 sdk.Cursor 分页并交给 consume；分页语义由 internal/pagination 拥有。
func Traverse[T any](ctx context.Context, plan command.Plan, fetch func(context.Context, sdk.Cursor) (sdk.Page[T], error), consume func([]T) error) error {
	return command.PageItems(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]T, sdk.Cursor, error) {
		page, err := fetch(ctx, cursor)
		return page.Items, page.Next, err
	}, consume)
}

// Run 统一 FANBOX 列表命令的 JSON / NDJSON / 文本输出与分页。JSON 只有完整文档
// 写入临时文件成功后才交给 stdout。
func Run[T any](ctx context.Context, out io.Writer, plan command.Plan, jsonKey string, jsonOut, ndjson bool, printText func([]T) error, toJSON func(T) any, fetch func(context.Context, sdk.Cursor) (sdk.Page[T], error)) error {
	if jsonOut {
		spool, err := command.NewJSONArraySpool(jsonKey)
		if err != nil {
			return err
		}
		defer spool.Close()
		if err := Traverse(ctx, plan, fetch, func(items []T) error {
			values := make([]any, len(items))
			for index, item := range items {
				values[index] = toJSON(item)
			}
			return command.AppendJSONArray(spool, values)
		}); err != nil {
			return err
		}
		return spool.Commit(out)
	}
	if ndjson {
		encoder := json.NewEncoder(out)
		return Traverse(ctx, plan, fetch, func(items []T) error {
			for _, item := range items {
				if err := encoder.Encode(toJSON(item)); err != nil {
					return err
				}
			}
			return nil
		})
	}
	return Traverse(ctx, plan, fetch, printText)
}
