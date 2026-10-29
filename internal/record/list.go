package record

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

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

// JSONKey 指定列表 JSON 文档的顶层键；这些键属于已发布的输出契约，不能改名。
type JSONKey string

const (
	// JSONKeyIllusts 指定作品列表的默认顶层键。
	JSONKeyIllusts JSONKey = "illusts"
	// JSONKeyManga 指定漫画排行榜等只返回漫画的列表使用的顶层键。
	JSONKeyManga JSONKey = "manga"
	// JSONKeyNovels 指定小说列表的顶层键。
	JSONKeyNovels JSONKey = "novels"
	// JSONKeyUsers 指定用户预览列表的顶层键。
	JSONKeyUsers JSONKey = "user_previews"
)

// JSONArraySpool 把单键列表文档的每个已确认元素写进私有临时文件；网络或编码失败时
// stdout 保持为空，Commit 之后一次性写出完整 JSON。
type JSONArraySpool struct {
	file  *os.File
	first bool
}

// NewJSONArraySpool 创建临时文件并写入文档开头 `{"key": [`。
func NewJSONArraySpool(key string) (*JSONArraySpool, error) {
	file, err := os.CreateTemp("", "pixiv-cli-json-*.tmp")
	if err != nil {
		return nil, err
	}
	if _, err := fmt.Fprintf(file, "{\n  %q: [", key); err != nil {
		discardTemp(file)
		return nil, err
	}
	return &JSONArraySpool{file: file, first: true}, nil
}

// AppendJSONArray 把一批元素按两空格缩进追加进数组。
func AppendJSONArray[T any](s *JSONArraySpool, items []T) error {
	for _, item := range items {
		if s.first {
			s.first = false
		} else if _, err := io.WriteString(s.file, ","); err != nil {
			return err
		}
		encoded, err := json.MarshalIndent(item, "    ", "  ")
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(s.file, "\n    %s", encoded); err != nil {
			return err
		}
	}
	return nil
}

// Commit 写完文档结尾，并把完整内容复制到 out。
func (s *JSONArraySpool) Commit(out io.Writer) error {
	if _, err := io.WriteString(s.file, "\n  ]\n}\n"); err != nil {
		return err
	}
	if _, err := s.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	_, err := io.Copy(out, s.file)
	return err
}

// Close 关闭并删除临时文件；无论 Commit 是否执行都调用。
func (s *JSONArraySpool) Close() {
	if s == nil {
		return
	}
	discardTemp(s.file)
}

func discardTemp(file *os.File) {
	name := file.Name()
	_ = file.Close()
	_ = os.Remove(name)
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
	body = append(body, '\n')
	_, err = out.Write(body)
	return err
}

// List 描述一次分页列表的输出方式；Key 为空时使用 JSONKeyIllusts。
type List[T any] struct {
	// Heading 在账号池选定账号之后、第一次输出之前求值，因此可以读取当前身份。
	Heading func() string
	Key     JSONKey
	// Print 输出一批文本结果；offset 是本批在整体结果中的起点。
	Print func(items []T, offset int) error
	// JSONValue 把一项转成 JSON 文档里的元素。
	JSONValue func(T) (any, error)
	// RecordValue 把一项转成 NDJSON 记录。
	RecordValue func(T) (any, error)
}

// writer 按 Format 写出一批分页结果。JSON 先写私有临时文档，失败时 stdout 保持为空；
// NDJSON 逐条提交；文本在第一批可见后提交。
type writer[T any] struct {
	out            io.Writer
	format         Format
	list           List[T]
	spool          *JSONArraySpool
	headingWritten bool
	position       int
	encoder        *json.Encoder
}

func newWriter[T any](out io.Writer, format Format, position int, list List[T]) *writer[T] {
	if list.Key == "" {
		list.Key = JSONKeyIllusts
	}
	return &writer[T]{out: out, format: format, list: list, position: position, encoder: json.NewEncoder(out)}
}

// begin 在每次重放尝试开始时重建 JSON 暂存，避免失败尝试的条目混入成功结果。
func (w *writer[T]) begin() error {
	if w.format != FormatJSON {
		return nil
	}
	w.spool.Close()
	spool, err := NewJSONArraySpool(string(w.list.Key))
	w.spool = spool
	return err
}

func (w *writer[T]) consume(items []T) (bool, error) {
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
			value, err := w.list.JSONValue(item)
			if err != nil {
				return false, err
			}
			values = append(values, value)
		}
		return false, AppendJSONArray(w.spool, values)
	default:
		committed := false
		if !w.headingWritten && w.list.Heading != nil {
			if text := w.list.Heading(); text != "" {
				committed = true
				if _, err := fmt.Fprintln(w.out, text); err != nil {
					return committed, err
				}
			}
			w.headingWritten = true
		}
		if err := w.list.Print(items, w.position); err != nil {
			return true, err
		}
		w.position += len(items)
		return true, nil
	}
}

func (w *writer[T]) commit() error {
	if w.format != FormatJSON {
		return nil
	}
	return w.spool.Commit(w.out)
}

// ArtworkList 描述一次作品列表读取及输出方式。
type ArtworkList struct {
	Heading func() string
	Key     JSONKey
	Fetch   func(*pixiv.Client, context.Context, sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error)
	// PostFilter 只在已解析的窗口内本地筛选，不会继续向上游多取数据。
	PostFilter func([]pixiv.Artwork) []pixiv.Artwork
	Print      func(items []pixiv.Artwork, start int) error
}

// NovelList 描述一次小说列表读取及输出方式。
type NovelList struct {
	Heading string
	Fetch   func(*pixiv.Client, context.Context, sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error)
	Print   func(items []pixiv.Novel) error
}

// UserList 描述一次用户预览列表读取及输出方式。
type UserList struct {
	Heading func() string
	Fetch   func(*pixiv.Client, context.Context, sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error)
	Print   func(items []pixiv.UserPreview) error
}

// StaticHeading 把固定标题包装成每次调用都返回同一文本的 Heading。
func StaticHeading(heading string) func() string { return func() string { return heading } }

// RunArtworks 在账号池重放边界内执行一次分页作品读取。NDJSON 逐条提交，JSON 在完整
// 文档写入 stdout 后提交，文本在第一批可见后提交；因此 429 只会在结果暴露前换账号。
func RunArtworks(ctx context.Context, runner session.Runner, request session.ProxyRequest, plan pagination.PagePlan, format Format, list ArtworkList) error {
	return runPixivList(ctx, runner, request, plan, format, List[pixiv.Artwork]{
		Heading:     list.Heading,
		Key:         list.Key,
		Print:       list.Print,
		JSONValue:   identityValue[pixiv.Artwork],
		RecordValue: artworkRecord,
	}, list.Fetch, list.PostFilter)
}

// RunNovels 在同一个重放边界内执行一次分页小说读取。
func RunNovels(ctx context.Context, runner session.Runner, request session.ProxyRequest, plan pagination.PagePlan, format Format, list NovelList) error {
	return runPixivList(ctx, runner, request, plan, format, List[pixiv.Novel]{
		Heading:     StaticHeading(list.Heading),
		Key:         JSONKeyNovels,
		Print:       func(items []pixiv.Novel, _ int) error { return list.Print(items) },
		JSONValue:   identityValue[pixiv.Novel],
		RecordValue: novelRecord,
	}, list.Fetch, nil)
}

// RunUsers 执行一次分页用户预览读取。标题只在第一次可见输出前求值，让身份读取仍可重放。
func RunUsers(ctx context.Context, runner session.Runner, request session.ProxyRequest, plan pagination.PagePlan, format Format, list UserList) error {
	return runPixivList(ctx, runner, request, plan, format, List[pixiv.UserPreview]{
		Heading:     list.Heading,
		Key:         JSONKeyUsers,
		Print:       func(items []pixiv.UserPreview, _ int) error { return list.Print(items) },
		JSONValue:   identityValue[pixiv.UserPreview],
		RecordValue: userPreviewRecord,
	}, list.Fetch, nil)
}

func runPixivList[T any](ctx context.Context, runner session.Runner, request session.ProxyRequest, plan pagination.PagePlan, format Format, list List[T], fetch func(*pixiv.Client, context.Context, sdk.Cursor) ([]T, sdk.Cursor, error), postFilter func([]T) []T) error {
	w := newWriter(runner.Out, format, plan.Skip, list)
	defer w.spool.Close()
	_, err := pagination.TraverseWith(ctx, runner.Execute(request), plan, w.begin,
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
	return w.commit()
}

// RunPages 统一 FANBOX 列表命令的 JSON / NDJSON / 文本输出与分页。JSON 只有完整文档
// 写入临时文件成功后才交给 stdout。
func RunPages[T any](ctx context.Context, out io.Writer, plan pagination.PagePlan, key string, jsonOut, ndjson bool, printText func([]T) error, toJSON func(T) any, fetch func(context.Context, sdk.Cursor) (sdk.Page[T], error)) error {
	format := FormatText
	if ndjson {
		format = FormatNDJSON
	} else if jsonOut {
		format = FormatJSON
	}
	value := func(item T) (any, error) { return toJSON(item), nil }
	w := newWriter(out, format, 0, List[T]{
		Key:         JSONKey(key),
		Print:       func(items []T, _ int) error { return printText(items) },
		JSONValue:   value,
		RecordValue: value,
	})
	if err := w.begin(); err != nil {
		return err
	}
	defer w.spool.Close()
	err := pagination.PageItems(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]T, sdk.Cursor, error) {
		page, err := fetch(ctx, cursor)
		return page.Items, page.Next, err
	}, func(items []T) error {
		_, err := w.consume(items)
		return err
	})
	if err != nil {
		return err
	}
	return w.commit()
}

func identityValue[T any](item T) (any, error) { return item, nil }

func artworkRecord(item pixiv.Artwork) (any, error) { return RecordFromArtwork(item) }

func novelRecord(item pixiv.Novel) (any, error) { return RecordFromNovel(item) }

func userPreviewRecord(item pixiv.UserPreview) (any, error) { return RecordFromUserPreview(item) }
