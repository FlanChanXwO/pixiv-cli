// Package listing owns Pixiv's shared paged-read execution and its bounded
// output commit semantics. Command packages retain flags and text presenters.
package listing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/FlanChanXwO/pixiv-cli/internal/shared/pagination"
	record "github.com/FlanChanXwO/pixiv-cli/internal/shared/record"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/traversal"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

// Plan is the resolved logical page selection shared by Pixiv list commands.
type Plan struct {
	limit    int
	skip     int
	oneBatch bool
}

// Request 是一次 Pixiv data execution 的传输覆写快照。listing 只传递它，
// 不创建 client，也不依赖 CLI resource graph。
type Request struct {
	UserID             int64
	HTTPSProxyOverride *string
}

// Executor 是 listing 需要的唯一资源端口；账号选择、SDK 构造和重放策略由
// composition root 注入，listing 只负责把分页结果交给该端口。
type Executor func(context.Context, Request, func(context.Context, *pixiv.Client) (bool, error)) error

// PagePlan exposes the resolved selection to command owners that call a search
// workflow directly instead of one of the paged runners here.
func (p Plan) PagePlan() pagination.PagePlan {
	return pagination.PagePlan{Skip: p.skip, Limit: p.limit, OneBatch: p.oneBatch}
}

// BindNDJSONFlag adds the record-stream output switch owned by a list command.
func BindNDJSONFlag(cmd *cobra.Command, value *bool) {
	cmd.Flags().BoolVar(value, "ndjson", false, "print one Pixiv entity record as JSON per line")
}

// BindListFlags adds logical pagination flags to one list command.
func BindListFlags(cmd *cobra.Command, limit, page *int) {
	flags := cmd.Flags()
	flags.SetOutput(cmd.ErrOrStderr())
	flags.IntVarP(limit, "limit", "l", 0, "maximum results; omitted returns one upstream batch; 0 returns all results")
	flags.IntVarP(page, "page", "p", 0, "1-based logical page (requires --limit > 0)")
}

// ParsePlan validates a command's logical pagination settings.
func ParsePlan(cmd *cobra.Command, limit, page int) (Plan, error) {
	if limit < 0 {
		return Plan{}, errors.New("limit must be zero or a positive integer")
	}
	if cmd.Flags().Changed("page") && page <= 0 {
		return Plan{}, errors.New("page must be a positive integer")
	}
	if page > 0 {
		if limit <= 0 {
			return Plan{}, errors.New("--page requires --limit to be a positive integer")
		}
		if page-1 > math.MaxInt/limit {
			return Plan{}, errors.New("page and limit overflow the logical result offset")
		}
		return Plan{limit: limit, skip: (page - 1) * limit}, nil
	}
	return Plan{limit: limit, oneBatch: !cmd.Flags().Changed("limit")}, nil
}

// Runner executes one paged read with the SDK ports selected by a command
// owner. It never owns account selection, retry or presentation policy.
type Runner struct {
	out      io.Writer
	executor Executor
}

func New(out io.Writer, executor Executor) Runner {
	return Runner{out: out, executor: executor}
}

// Executor 把一次 request 绑定为共享 traversal 的可重入执行函数；逻辑分页、
// bookmark 策略和 cursor 处理仍由各 adapter 拥有。
func (a Runner) Executor(request Request) traversal.Execute[*pixiv.Client] {
	if a.executor == nil {
		return nil
	}
	return func(ctx context.Context, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
		return a.executor(ctx, request, attempt)
	}
}

func runPages[C any, T any](ctx context.Context, execute traversal.Execute[C], plan pagination.PagePlan, begin func() error, fetch func(context.Context, C, sdk.Cursor) ([]T, sdk.Cursor, error), consume func([]T) (bool, error)) error {
	if execute == nil {
		return sdk.NewError("pixiv", "PagedRead", sdk.LocalStateError,
			sdk.WithDetail("sdk pooled operation is not configured"))
	}
	return traversal.TraverseWith(ctx, execute, plan, begin, fetch, consume)
}

// pageItems 只把 CLI 的分页计划交给共享算法；cursor 遍历、跳过、限量与止环
// 均不属于 command owner。
func PageItems[T any](ctx context.Context, plan Plan, fetch func(context.Context, sdk.Cursor) ([]T, sdk.Cursor, error), consume func([]T) error) error {
	_, err := pagination.TraversePages(ctx, plan.PagePlan(), fetch, consume)
	return err
}

// RunPooledIllustList 将一个内容读取收敛在账号池的安全重放边界。NDJSON 在每条
// record 成功写出后提交；JSON 在完整临时文档成功交给 stdout 后提交；文本在首批
// 可见作品写出后提交。因而 429 只会在未向用户暴露任何结果时切换本地账号。
func (a Runner) RunPooledIllustList(ctx context.Context, request Request, plan Plan, jsonOut, ndjson bool, heading string, fetch func(*pixiv.Client, context.Context, sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error), print func([]pixiv.Artwork, int) error) error {
	return a.RunPooledIllustListWithHeading(ctx, request, plan, jsonOut, ndjson, func() string { return heading }, fetch, print)
}

// RunPooledIllustListWithHeading 允许身份由当前池内账号决定的读取在真正输出前再生成标题。
func (a Runner) RunPooledIllustListWithHeading(ctx context.Context, request Request, plan Plan, jsonOut, ndjson bool, heading func() string, fetch func(*pixiv.Client, context.Context, sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error), print func([]pixiv.Artwork, int) error) error {
	return a.RunPooledIllustListWithKey(ctx, request, plan, jsonOut, ndjson, "illusts", heading, fetch, print)
}

// RunPooledIllustListWithKey 保留少数已有 JSON 兼容键（例如 manga），同时复用完整的
// 账号池提交边界。
func (a Runner) RunPooledIllustListWithKey(ctx context.Context, request Request, plan Plan, jsonOut, ndjson bool, jsonKey string, heading func() string, fetch func(*pixiv.Client, context.Context, sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error), print func([]pixiv.Artwork, int) error) error {
	var spool *jsonArraySpool
	if jsonOut {
		defer func() {
			if spool != nil {
				spool.Close()
			}
		}()
	}
	begin := func() error {
		if !jsonOut {
			return nil
		}
		if spool != nil {
			spool.Close()
		}
		var err error
		spool, err = newJSONArraySpool(jsonKey)
		return err
	}
	position := plan.skip
	headingWritten := false
	encoder := json.NewEncoder(a.out)
	err := runPages(ctx, a.Executor(request), plan.PagePlan(), begin,
		func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
			return fetch(client, ctx, cursor)
		}, func(items []pixiv.Artwork) (bool, error) {
			if ndjson {
				committed := false
				for _, item := range items {
					record, err := record.RecordFromArtworkDTO(pixiv.ToArtworkDTO(item))
					if err != nil {
						return committed, err
					}
					committed = true
					if err := encoder.Encode(record); err != nil {
						return committed, err
					}
				}
				return committed, nil
			}
			if jsonOut {
				return false, appendJSONArray(spool, artworkDTOs(items))
			}
			committed := false
			if !headingWritten && heading != nil {
				if text := heading(); text != "" {
					committed = true
					if _, err := fmt.Fprintln(a.out, text); err != nil {
						return committed, err
					}
				}
				headingWritten = true
			}
			if err := print(items, position); err != nil {
				return true, err
			}
			position += len(items)
			return true, nil
		})
	if jsonOut && err == nil {
		return spool.Commit(a.out)
	}
	return err
}

// RunPooledNovelList executes a paged novel read under the same replay boundary.
func (a Runner) RunPooledNovelList(ctx context.Context, request Request, plan Plan, jsonOut, ndjson bool, heading string, fetch func(*pixiv.Client, context.Context, sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error), print func([]pixiv.Novel) error) error {
	var spool *jsonArraySpool
	if jsonOut {
		defer func() {
			if spool != nil {
				spool.Close()
			}
		}()
	}
	begin := func() error {
		if !jsonOut {
			return nil
		}
		if spool != nil {
			spool.Close()
		}
		var err error
		spool, err = newJSONArraySpool("novels")
		return err
	}
	headingWritten := false
	encoder := json.NewEncoder(a.out)
	err := runPages(ctx, a.Executor(request), plan.PagePlan(), begin,
		func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
			return fetch(client, ctx, cursor)
		}, func(items []pixiv.Novel) (bool, error) {
			if ndjson {
				committed := false
				for _, item := range items {
					record, err := record.RecordFromNovelDTO(pixiv.ToNovelDTO(item))
					if err != nil {
						return committed, err
					}
					committed = true
					if err := encoder.Encode(record); err != nil {
						return committed, err
					}
				}
				return committed, nil
			}
			if jsonOut {
				return false, appendJSONArray(spool, novelDTOs(items))
			}
			committed := false
			if !headingWritten && heading != "" {
				committed = true
				if _, err := fmt.Fprintln(a.out, heading); err != nil {
					return committed, err
				}
				headingWritten = true
			}
			if err := print(items); err != nil {
				return true, err
			}
			return true, nil
		})
	if jsonOut && err == nil {
		return spool.Commit(a.out)
	}
	return err
}

// RunPooledUserList 在首批实际输出前才求值标题，使取当前账号身份等前置读取也位于
// 可安全重放的边界内。
func (a Runner) RunPooledUserList(ctx context.Context, request Request, plan Plan, jsonOut, ndjson bool, heading func() string, fetch func(*pixiv.Client, context.Context, sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error), print func([]pixiv.UserPreview) error) error {
	var spool *jsonArraySpool
	if jsonOut {
		defer func() {
			if spool != nil {
				spool.Close()
			}
		}()
	}
	begin := func() error {
		if !jsonOut {
			return nil
		}
		if spool != nil {
			spool.Close()
		}
		var err error
		spool, err = newJSONArraySpool("user_previews")
		return err
	}
	headingWritten := false
	encoder := json.NewEncoder(a.out)
	err := runPages(ctx, a.Executor(request), plan.PagePlan(), begin,
		func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error) {
			return fetch(client, ctx, cursor)
		}, func(items []pixiv.UserPreview) (bool, error) {
			if ndjson {
				committed := false
				for _, item := range items {
					record, err := record.RecordFromUserPreviewDTO(pixiv.ToUserPreviewDTO(item))
					if err != nil {
						return committed, err
					}
					committed = true
					if err := encoder.Encode(record); err != nil {
						return committed, err
					}
				}
				return committed, nil
			}
			if jsonOut {
				return false, appendJSONArray(spool, userPreviewDTOs(items))
			}
			committed := false
			if !headingWritten && heading != nil {
				if text := heading(); text != "" {
					committed = true
					if _, err := fmt.Fprintln(a.out, text); err != nil {
						return committed, err
					}
				}
				headingWritten = true
			}
			if err := print(items); err != nil {
				return true, err
			}
			return true, nil
		})
	if jsonOut && err == nil {
		return spool.Commit(a.out)
	}
	return err
}

// printUserPreviews 保留文本列表的一行一个用户格式，并将 stdout 失败交给调用方。
func printUserPreviews(w io.Writer, users []pixiv.UserPreview) error {
	for _, item := range users {
		if _, err := fmt.Fprintf(w, "%d %s\n", item.User.ID, item.User.Name); err != nil {
			return err
		}
	}
	return nil
}

func artworkDTOs(items []pixiv.Artwork) []pixiv.ArtworkDTO {
	out := make([]pixiv.ArtworkDTO, 0, len(items))
	for _, item := range items {
		out = append(out, pixiv.ToArtworkDTO(item))
	}
	return out
}

func novelDTOs(items []pixiv.Novel) []pixiv.NovelDTO {
	out := make([]pixiv.NovelDTO, 0, len(items))
	for _, item := range items {
		out = append(out, pixiv.ToNovelDTO(item))
	}
	return out
}

func userPreviewDTOs(items []pixiv.UserPreview) []pixiv.UserPreviewDTO {
	out := make([]pixiv.UserPreviewDTO, 0, len(items))
	for _, item := range items {
		out = append(out, pixiv.ToUserPreviewDTO(item))
	}
	return out
}

// jsonArraySpool 将已确认成功的每个元素逐个写入私有临时文件。网络或编码失败时
// stdout 完全不写；成功后才把一个完整 JSON document 原子性地转交给调用方 writer。
type jsonArraySpool struct {
	file  *os.File
	first bool
	key   string
}

func newJSONArraySpool(key string) (*jsonArraySpool, error) {
	file, err := os.CreateTemp("", "pixiv-cli-json-*.tmp")
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

// marshalJSONValue 统一分页 JSON spool 与普通 Pixiv JSON 输出入口；调用方在
// 到达这里前必须把运行时 SDK 模型转换为 DTO。
func marshalJSONValue(value any, indent bool) ([]byte, error) {
	if indent {
		return json.MarshalIndent(value, "    ", "  ")
	}
	return json.Marshal(value)
}

// writeJSONLine 统一实体之外的 NDJSON 输出入口。结构化调用方负责传入 DTO、
// Record、envelope 或 scalar，而不是运行时 SDK 模型。
func writeJSONLine(out io.Writer, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := out.Write(body); err != nil {
		return err
	}
	_, err = io.WriteString(out, "\n")
	return err
}

func appendJSONArray[T any](s *jsonArraySpool, items []T) error {
	for _, item := range items {
		if s.first {
			s.first = false
		} else if _, err := io.WriteString(s.file, ","); err != nil {
			return err
		}
		encoded, err := marshalJSONValue(item, true)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(s.file, "\n    %s", encoded); err != nil {
			return err
		}
	}
	return nil
}

func (s *jsonArraySpool) Commit(out io.Writer) error {
	return s.commit(out, "", "")
}

func (s *jsonArraySpool) commit(out io.Writer, extraName, extraValue string) error {
	if _, err := io.WriteString(s.file, "\n  ]"); err != nil {
		return err
	}
	if extraName != "" {
		encodedName, err := json.Marshal(extraName)
		if err != nil {
			return err
		}
		encodedValue, err := json.Marshal(extraValue)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(s.file, ",\n  %s: %s", encodedName, encodedValue); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(s.file, "\n}\n"); err != nil {
		return err
	}
	if _, err := s.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	_, err := io.Copy(out, s.file)
	return err
}

func (s *jsonArraySpool) Close() {
	if s == nil || s.file == nil {
		return
	}
	name := s.file.Name()
	_ = s.file.Close()
	_ = os.Remove(name)
}
