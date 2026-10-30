package record

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// Format 指定列表命令的输出格式，由 --json/--ndjson 决定；两个 flag 互斥。
type Format int

const (
	// FormatText 输出面向终端的逐行文本。
	FormatText Format = iota
	// FormatJSON 输出一个完整 JSON 文档。
	FormatJSON
	// FormatNDJSON 每条记录输出一行 JSON。
	FormatNDJSON
)

// FormatOf 把两个输出 flag 映射成一种列表格式，--ndjson 优先于 --json。
func FormatOf(jsonOut, ndjson bool) Format {
	if ndjson {
		return FormatNDJSON
	}
	if jsonOut {
		return FormatJSON
	}
	return FormatText
}

// WriteJSONLine 输出一个紧凑 JSON 值并追加换行。
func WriteJSONLine(out io.Writer, value any) error {
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

// WriteJSONValue 输出一个两空格缩进的 JSON 值并追加换行。
func WriteJSONValue(out io.Writer, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	spool, err := NewSpool()
	if err != nil {
		return err
	}
	defer spool.Close()
	if _, err := spool.Write(append(body, '\n')); err != nil {
		return err
	}
	return spool.Commit(out)
}

// List 描述一次分页列表的输出方式；JSON 与 NDJSON 共用记录投影。
type List[T any] struct {
	// Print 输出一批文本结果；offset 是本批在整体结果中的起点。
	Print func(items []T, offset int) error
	// RecordValue 把一项转成 JSON/NDJSON 共用记录。
	RecordValue func(T) (any, error)
}

// writer 按 Format 写出一批分页结果。JSON 先写私有临时文档，失败时 stdout 保持为空；
// NDJSON 逐条提交；文本在第一批可见后提交。
type writer[T any] struct {
	out      io.Writer
	format   Format
	list     List[T]
	spool    *JSONArraySpool
	returned int
	position int
	encoder  *json.Encoder
}

func newWriter[T any](out io.Writer, format Format, position int, list List[T]) *writer[T] {
	return &writer[T]{out: out, format: format, list: list, position: position, encoder: json.NewEncoder(out)}
}

// begin 在每次重放尝试开始时重建 JSON 暂存，避免失败尝试的条目混入成功结果。
func (w *writer[T]) begin() error {
	if w.format != FormatJSON {
		return nil
	}
	w.spool.Close()
	w.returned = 0
	spool, err := NewJSONArraySpool()
	w.spool = spool
	return err
}

func (w *writer[T]) consume(items []T) (bool, error) {
	w.returned += len(items)
	switch w.format {
	case FormatNDJSON:
		committed := false
		for _, item := range items {
			value, err := w.list.RecordValue(item)
			if err != nil {
				return committed, err
			}
			committed = true
			if err := w.encoder.Encode(value); err != nil {
				return committed, err
			}
		}
		return committed, nil
	case FormatJSON:
		values := make([]any, 0, len(items))
		for _, item := range items {
			value, err := w.list.RecordValue(item)
			if err != nil {
				return false, err
			}
			values = append(values, value)
		}
		return false, AppendJSONArray(w.spool, values)
	default:
		if len(items) == 0 {
			return false, nil
		}
		if err := w.list.Print(items, w.position); err != nil {
			return true, err
		}
		w.position += len(items)
		return true, nil
	}
}

// closeSpool 延迟求值，保证 begin 在每次重放尝试中新建的临时文档也会被清理。
func (w *writer[T]) closeSpool() { w.spool.Close() }

func (w *writer[T]) commit(plan pagination.PagePlan, result pagination.PageResult) error {
	if w.format != FormatJSON {
		return nil
	}
	result.Returned = w.returned
	return w.spool.CommitList(w.out, PageMetadata(plan, result))
}

// ArtworkList 描述一次作品列表读取及输出方式。
type ArtworkList struct {
	Fetch func(*pixiv.Client, context.Context, sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error)
	// PostFilter 只在已解析的窗口内本地筛选，不会继续向上游多取数据。
	PostFilter func([]pixiv.Artwork) []pixiv.Artwork
	Print      func(items []pixiv.Artwork, start int) error
}

// NovelList 描述一次小说列表读取及输出方式。
type NovelList struct {
	Fetch func(*pixiv.Client, context.Context, sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error)
	Print func(items []pixiv.Novel) error
}

// UserList 描述一次用户预览列表读取及输出方式。
type UserList struct {
	Fetch func(*pixiv.Client, context.Context, sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error)
	Print func(items []pixiv.UserPreview) error
}

// RunArtworks 在账号池重放边界内执行一次分页作品读取。NDJSON 逐条提交，JSON 在完整
// 文档写入 stdout 后提交，文本在第一批可见后提交；因此 429 只会在结果暴露前换账号。
func RunArtworks(ctx context.Context, runner session.Runner, request session.ProxyRequest, plan pagination.PagePlan, format Format, errOut io.Writer, list ArtworkList) error {
	return runPixivList(ctx, runner, request, plan, format, errOut, List[pixiv.Artwork]{
		Print:       list.Print,
		RecordValue: artworkRecord,
	}, list.Fetch, list.PostFilter)
}

// RunNovels 在同一个重放边界内执行一次分页小说读取。
func RunNovels(ctx context.Context, runner session.Runner, request session.ProxyRequest, plan pagination.PagePlan, format Format, errOut io.Writer, list NovelList) error {
	return runPixivList(ctx, runner, request, plan, format, errOut, List[pixiv.Novel]{
		Print:       func(items []pixiv.Novel, _ int) error { return list.Print(items) },
		RecordValue: novelRecord,
	}, list.Fetch, nil)
}

// RunUsers 执行一次分页用户预览读取。
func RunUsers(ctx context.Context, runner session.Runner, request session.ProxyRequest, plan pagination.PagePlan, format Format, errOut io.Writer, list UserList) error {
	return runPixivList(ctx, runner, request, plan, format, errOut, List[pixiv.UserPreview]{
		Print:       func(items []pixiv.UserPreview, _ int) error { return list.Print(items) },
		RecordValue: userPreviewRecord,
	}, list.Fetch, nil)
}

func runPixivList[T any](ctx context.Context, runner session.Runner, request session.ProxyRequest, plan pagination.PagePlan, format Format, errOut io.Writer, list List[T], fetch func(*pixiv.Client, context.Context, sdk.Cursor) ([]T, sdk.Cursor, error), postFilter func([]T) []T) error {
	w := newWriter(runner.Out, format, plan.Skip, list)
	defer w.closeSpool()
	result, err := pagination.TraverseWith(ctx, runner.Execute(request), plan, w.begin,
		func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]T, sdk.Cursor, error) {
			return fetch(client, ctx, cursor)
		}, func(items []T) (bool, error) {
			if postFilter != nil {
				items = postFilter(items)
			}
			return w.consume(items)
		})
	if err != nil {
		return err
	}
	if format == FormatText && w.returned == 0 {
		_, err := fmt.Fprintln(errOut, "No results.")
		return err
	}
	return w.commit(plan, result)
}

// RunPages 统一 FANBOX 列表命令的 JSON / NDJSON / 文本输出与分页。JSON 只有完整文档
// 写入临时文件成功后才交给 stdout。
func RunPages[T any](ctx context.Context, out, errOut io.Writer, plan pagination.PagePlan, jsonOut, ndjson bool, printText func([]T) error, toJSON func(T) any, fetch func(context.Context, sdk.Cursor) (sdk.Page[T], error)) error {
	format := FormatText
	if ndjson {
		format = FormatNDJSON
	} else if jsonOut {
		format = FormatJSON
	}
	value := func(item T) (any, error) { return toJSON(item), nil }
	w := newWriter(out, format, 0, List[T]{
		Print:       func(items []T, _ int) error { return printText(items) },
		RecordValue: value,
	})
	if err := w.begin(); err != nil {
		return err
	}
	defer w.closeSpool()
	result, err := pagination.TraversePages(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]T, sdk.Cursor, error) {
		page, err := fetch(ctx, cursor)
		return page.Items, page.Next, err
	}, func(items []T) error {
		_, err := w.consume(items)
		return err
	})
	if err != nil {
		return err
	}
	if format == FormatText && w.returned == 0 {
		_, err := fmt.Fprintln(errOut, "No results.")
		return err
	}
	return w.commit(plan, result)
}

func artworkRecord(item pixiv.Artwork) (any, error) { return RecordFromArtwork(item) }

func novelRecord(item pixiv.Novel) (any, error) { return RecordFromNovel(item) }

func userPreviewRecord(item pixiv.UserPreview) (any, error) { return RecordFromUserPreview(item) }
