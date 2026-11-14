package record

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
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
	// PostFilter 只筛选已经解析的分页窗口，不额外获取上游结果。
	PostFilter func([]T) []T
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

func runList[E any, T any, C pagination.Cursor](ctx context.Context, out, errOut io.Writer, execute pagination.Execute[E], plan pagination.PagePlan, format Format, list List[T], fetch func(context.Context, E, C) ([]T, C, error)) error {
	w := newWriter(out, format, plan.Skip, list)
	defer w.closeSpool()
	result, err := pagination.TraverseWith(ctx, execute, plan, w.begin, fetch, func(items []T) (bool, error) {
		if list.PostFilter != nil {
			items = list.PostFilter(items)
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
		return page.Items, page.Cursor, err
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
