package pixivcmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	record "github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

// Format 是列表命令的输出格式，由 --json/--ndjson 决定；两个 flag 互斥。
type Format int

const (
	// FormatText 输出面向终端的逐行文本。
	FormatText Format = iota
	// FormatJSON 输出一个完整 JSON 文档。
	FormatJSON
	// FormatNDJSON 每条记录输出一行 JSON。
	FormatNDJSON
)

// JSONKey 是列表 JSON 文档的顶层键。这些键属于已发布的输出契约，不能改名。
type JSONKey string

const (
	// JSONKeyIllusts 是作品列表的默认顶层键。
	JSONKeyIllusts JSONKey = "illusts"
	// JSONKeyManga 是漫画排行榜等只返回漫画的列表使用的顶层键。
	JSONKeyManga JSONKey = "manga"
	// JSONKeyNovels 是小说列表的顶层键。
	JSONKeyNovels JSONKey = "novels"
	// JSONKeyUsers 是用户预览列表的顶层键。
	JSONKeyUsers JSONKey = "user_previews"
)

// ArtworkList 描述一次作品列表读取及输出方式。
type ArtworkList struct {
	// Heading 在账号池选定账号之后、第一次输出之前求值，因此可以读取当前身份。
	Heading func() string
	// Key 为空时使用 JSONKeyIllusts；少数命令保留自己的顶层键。
	Key JSONKey
	// Fetch 读取一批上游结果，游标由 SDK 返回。
	Fetch func(*pixiv.Client, context.Context, sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error)
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

// Static 把固定标题包装成每次调用都返回同一文本的 Heading。
func Static(heading string) func() string { return func() string { return heading } }

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

// RunArtworks 在账号池重放边界内执行一次分页作品读取。NDJSON 逐条提交，JSON 在完整
// 文档写入 stdout 后提交，文本在第一批可见后提交；因此 429 只会在结果暴露前换账号。
func (r Runner) RunArtworks(ctx context.Context, request Request, plan command.Plan, format Format, list ArtworkList) error {
	key := list.Key
	if key == "" {
		key = JSONKeyIllusts
	}
	return runList(ctx, r, request, plan, format, key, list.Heading, list.Fetch, list.PostFilter, list.Print, record.RecordFromArtwork)
}

// RunNovels 在同一个重放边界内执行一次分页小说读取。
func (r Runner) RunNovels(ctx context.Context, request Request, plan command.Plan, format Format, list NovelList) error {
	return runList(ctx, r, request, plan, format, JSONKeyNovels, Static(list.Heading), list.Fetch, nil, func(items []pixiv.Novel, _ int) error {
		return list.Print(items)
	}, record.RecordFromNovel)
}

// RunUsers 执行一次分页用户预览读取。标题只在第一次可见输出前求值，让身份读取仍可重放。
func (r Runner) RunUsers(ctx context.Context, request Request, plan command.Plan, format Format, list UserList) error {
	return runList(ctx, r, request, plan, format, JSONKeyUsers, list.Heading, list.Fetch, nil, func(items []pixiv.UserPreview, _ int) error {
		return list.Print(items)
	}, record.RecordFromUserPreview)
}

func runList[T any](ctx context.Context, r Runner, request Request, plan command.Plan, format Format, key JSONKey, heading func() string, fetch func(*pixiv.Client, context.Context, sdk.Cursor) ([]T, sdk.Cursor, error), postFilter func([]T) []T, print func([]T, int) error, toRecord func(T) (record.Record, error)) error {
	var spool *command.JSONArraySpool
	if format == FormatJSON {
		defer func() {
			if spool != nil {
				spool.Close()
			}
		}()
	}
	begin := func() error {
		if format != FormatJSON {
			return nil
		}
		if spool != nil {
			spool.Close()
		}
		var err error
		spool, err = command.NewJSONArraySpool(string(key))
		return err
	}
	position := plan.PagePlan().Skip
	headingWritten := false
	encoder := json.NewEncoder(r.out)
	_, err := pagination.TraverseWith(ctx, r.Execute(request), plan.PagePlan(), begin,
		func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]T, sdk.Cursor, error) {
			return fetch(client, ctx, cursor)
		}, func(items []T) (bool, error) {
			if postFilter != nil {
				items = postFilter(items)
			}
			if format == FormatNDJSON {
				committed := false
				for _, item := range items {
					value, err := toRecord(item)
					if err != nil {
						return committed, err
					}
					committed = true
					if err := encoder.Encode(value); err != nil {
						return committed, err
					}
				}
				return committed, nil
			}
			if format == FormatJSON {
				return false, command.AppendJSONArray(spool, items)
			}
			committed := false
			if !headingWritten && heading != nil {
				if text := heading(); text != "" {
					committed = true
					if _, err := fmt.Fprintln(r.out, text); err != nil {
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
	if format == FormatJSON && err == nil {
		return spool.Commit(r.out)
	}
	return err
}

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
