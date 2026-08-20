// Package listing owns the FANBOX shared paged-read execution and its bounded
// output commit semantics. Command packages keep their own flags and presenters.
package listing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"

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

// Plan is the resolved logical page selection shared by FANBOX list commands.
type Plan struct {
	limit    int
	skip     int
	oneBatch bool
}

// AllResults returns the plan used by workflows that must visit every page.
func AllResults() Plan { return Plan{} }

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
func ParsePlan(cmd *cobra.Command, opts Options) (Plan, error) {
	if opts.Limit < 0 {
		return Plan{}, errors.New("limit must be zero or a positive integer")
	}
	if cmd.Flags().Changed("page") && opts.Page <= 0 {
		return Plan{}, errors.New("page must be a positive integer")
	}
	if opts.Page > 0 {
		if opts.Limit <= 0 {
			return Plan{}, errors.New("--page requires --limit to be a positive integer")
		}
		if opts.Page-1 > math.MaxInt/opts.Limit {
			return Plan{}, errors.New("page and limit overflow the logical result offset")
		}
		return Plan{limit: opts.Limit, skip: (opts.Page - 1) * opts.Limit}, nil
	}
	return Plan{limit: opts.Limit, oneBatch: !cmd.Flags().Changed("limit")}, nil
}

// JSONOut resolves the document output mode and rejects the --ndjson/--json
// combination through the command's usage-error wrapper.
func JSONOut(cmd *cobra.Command, opts Options, usageError func(error) error) (bool, error) {
	if opts.NDJSON && cmd.Flags().Changed("json") {
		return false, usageError(errors.New("--ndjson cannot be used with --json"))
	}
	return !opts.NDJSON && opts.JSON, nil
}

// Traverse 跟随 sdk.Cursor 分页并交给 consume。成功空结果保持 non-nil；oneBatch
// 语义与 pixiv 列表一致（跳过空批直到首个非空逻辑结果或真正结束）。
func Traverse[T any](ctx context.Context, plan Plan, fetch func(context.Context, sdk.Cursor) (sdk.Page[T], error), consume func([]T) error) error {
	cursor := sdk.Cursor{}
	seen := make(map[string]struct{})
	returned := 0
	skip := plan.skip
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		key := cursor.String()
		if _, exists := seen[key]; exists {
			return errors.New("pagination cursor repeated")
		}
		seen[key] = struct{}{}
		page, err := fetch(ctx, cursor)
		if err != nil {
			return err
		}
		items := page.Items
		if skip >= len(items) {
			skip -= len(items)
			items = nil
		} else if skip > 0 {
			items = items[skip:]
			skip = 0
		}
		if plan.limit > 0 {
			remaining := plan.limit - returned
			if len(items) > remaining {
				items = items[:remaining]
			}
		}
		if len(items) > 0 {
			if err := consume(items); err != nil {
				return err
			}
			returned += len(items)
		}
		if plan.limit > 0 && returned >= plan.limit {
			return nil
		}
		if plan.oneBatch && (returned > 0 || page.Next.IsZero()) {
			return nil
		}
		if page.Next.IsZero() {
			return nil
		}
		cursor = page.Next
	}
}

// Run 统一 FANBOX 列表命令的 JSON / NDJSON / 文本输出与分页。JSON 只有完整文档
// 写入临时文件成功后才交给 stdout。
func Run[T any](ctx context.Context, out io.Writer, plan Plan, jsonKey string, jsonOut, ndjson bool, printText func([]T) error, toJSON func(T) any, fetch func(context.Context, sdk.Cursor) (sdk.Page[T], error)) error {
	if jsonOut {
		spool, err := newJSONArraySpool(jsonKey)
		if err != nil {
			return err
		}
		defer spool.Close()
		if err := Traverse(ctx, plan, fetch, func(items []T) error {
			return appendJSONArray(spool, items, toJSON)
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

// jsonArraySpool 使网络或编码失败时 stdout 保持为空；只有完整 JSON 文档写入临时
// 文件成功后，才把内容交给调用方 writer。
type jsonArraySpool struct {
	file  *os.File
	first bool
	key   string
}

func newJSONArraySpool(key string) (*jsonArraySpool, error) {
	file, err := os.CreateTemp("", "pixiv-cli-fanbox-json-*.tmp")
	if err != nil {
		return nil, err
	}
	spool := &jsonArraySpool{file: file, first: true, key: key}
	if _, err := fmt.Fprintf(file, "{\n  %q: [", key); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return nil, err
	}
	return spool, nil
}

func appendJSONArray[T any](spool *jsonArraySpool, items []T, toJSON func(T) any) error {
	for _, item := range items {
		if spool.first {
			spool.first = false
		} else if _, err := io.WriteString(spool.file, ","); err != nil {
			return err
		}
		encoded, err := json.MarshalIndent(toJSON(item), "    ", "  ")
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(spool.file, "\n    %s", encoded); err != nil {
			return err
		}
	}
	return nil
}

func (spool *jsonArraySpool) Commit(out io.Writer) error {
	if _, err := io.WriteString(spool.file, "\n  ]\n}\n"); err != nil {
		return err
	}
	if _, err := spool.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	_, err := io.Copy(out, spool.file)
	return err
}

func (spool *jsonArraySpool) Close() {
	if spool == nil || spool.file == nil {
		return
	}
	name := spool.file.Name()
	_ = spool.file.Close()
	_ = os.Remove(name)
}
