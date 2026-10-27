package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/toolhost"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/toolrun"
	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	record "github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (t pixivTools) registerBookmarkAdd(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_bookmark_add", Description: "Add an artwork or novel bookmark.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)},
		InputSchema: schemas.ClosedObject(map[string]any{"kind": schemas.EnumString("Target content type.", kindArtwork, kindNovel), "id": schemas.PositiveInteger("Positive artwork or novel ID."), "restrict": map[string]any{"type": "string", "description": "public or private; omitted or empty uses SDK default."}, "tags": map[string]any{"type": []string{"array", "null"}, "items": map[string]any{"type": "string"}}}, []string{"kind", "id"})}, func(ctx context.Context, request *mcp.CallToolRequest, input bookmarkAddIn) (*mcp.CallToolResult, outputs.Mutation, error) {
		return t.bookmarkAddHandler(ctx, input)
	})
}

// bookmarkAddIn 的 kind 决定写入作品还是小说收藏；输出沿用原操作的 mutation envelope。
type bookmarkAddIn struct {
	Kind     string   `json:"kind"`
	ID       int64    `json:"id" jsonschema:"artwork ID"`
	Restrict string   `json:"restrict,omitempty" jsonschema:"public or private"`
	Tags     []string `json:"tags,omitempty" jsonschema:"bookmark tags"`
}

func (t pixivTools) bookmarkAddHandler(ctx context.Context, in bookmarkAddIn) (*mcp.CallToolResult, outputs.Mutation, error) {
	if in.Kind == kindNovel {
		out := outputs.Mutation{Action: "add_novel_bookmark", NovelID: in.ID, Text: fmt.Sprintf("Bookmarked novel %d.", in.ID)}
		return outputs.RunMutation(&out, func() error {
			return toolhost.Write(t.app, ctx, func(ctx context.Context, client *pixiv.Client) error {
				return client.AddNovelBookmark(ctx, pixiv.AddNovelBookmarkRequest{NovelID: in.ID, Restrict: pixiv.Restrict(in.Restrict), Tags: in.Tags})
			})
		})
	}

	out := outputs.Mutation{Action: "add_bookmark", IllustID: in.ID, Text: fmt.Sprintf("Bookmarked artwork %d.", in.ID)}
	return outputs.RunMutation(&out, func() error {
		return toolhost.Write(t.app, ctx, func(ctx context.Context, client *pixiv.Client) error {
			return client.AddArtworkBookmark(ctx, pixiv.AddArtworkBookmarkRequest{ArtworkID: in.ID, Restrict: pixiv.Restrict(in.Restrict), Tags: in.Tags})
		})
	})
}
func (t pixivTools) registerBookmarkDetail(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_bookmark_detail", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "Get the current user's bookmark state for one artwork.", OutputSchema: schemas.BookmarkDetailOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input bookmarkDetailIn) (*mcp.CallToolResult, outputs.BookmarkDetail, error) {
		return t.bookmarkDetailHandler(ctx, input)
	})
}

type bookmarkDetailIn struct {
	IllustID int64 `json:"illust_id" jsonschema:"positive artwork ID"`
}

func (t pixivTools) bookmarkDetailHandler(ctx context.Context, in bookmarkDetailIn) (*mcp.CallToolResult, outputs.BookmarkDetail, error) {
	if in.IllustID <= 0 {
		return outputs.BookmarkDetailError(errors.New("illust_id must be a positive integer"))
	}
	result, err := toolhost.Read(t.app, ctx, func(ctx context.Context, client *pixiv.Client) (pixiv.ArtworkBookmarkDetail, error) {
		return client.ArtworkBookmark(ctx, pixiv.ArtworkBookmarkRequest{ArtworkID: in.IllustID})
	})
	if err != nil {
		return outputs.BookmarkDetailError(err)
	}
	out := outputs.BookmarkDetail{Bookmarked: result.Restrict != "", Restrict: string(result.Restrict), Tags: append([]string{}, result.Tags...)}
	return outputs.BookmarkDetailResult(out, in.IllustID), out, nil
}

// registerBookmarkListAll 注册 bookmark_list_all，把作品与小说收藏合并成一个序列；
// user_bookmarks 与 user_novel_bookmarks 的 wire 契约保持不变。
func (t pixivTools) registerBookmarkListAll(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{
		Name: "pixiv_bookmark_list_all", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)},
		Description: "Browse artwork bookmarks followed by novel bookmarks with one logical budget.",
		InputSchema: schemas.List(map[string]any{
			"user_id":  schemas.PositiveInteger("Optional positive Pixiv user ID; defaults to the authenticated user."),
			"restrict": schemas.EnumString("Bookmark visibility.", string(pixiv.RestrictPublic), string(pixiv.RestrictPrivate)),
			"tag":      map[string]any{"type": "string", "description": "Optional bookmark tag."},
		}),
		OutputSchema: schemas.RecordsOutputSchema(),
	}, func(ctx context.Context, request *mcp.CallToolRequest, input bookmarkListAllIn) (*mcp.CallToolResult, outputs.Records, error) {
		return t.bookmarkListAllHandler(ctx, input)
	})
}

// bookmarkListAllIn 定义 bookmark_list_all 的封闭输入；restrict 与 tag 同时作用于两类
// 收藏。
type bookmarkListAllIn struct {
	UserID   int64  `json:"user_id,omitempty"`
	Restrict string `json:"restrict,omitempty"`
	Tag      string `json:"tag,omitempty"`
	toolrun.ListIn
}

type bookmarkCursor struct {
	upstream sdk.Cursor
	consumed int
}

// IsZero 报告是否既没有消费批内条目，也没有上游 cursor。
func (c bookmarkCursor) IsZero() bool { return c.consumed == 0 && c.upstream.IsZero() }

// String 把上游 cursor 与已消费条数序列化成稳定的 cursor 文本；零值返回空串。
func (c bookmarkCursor) String() string {
	if c.IsZero() {
		return ""
	}
	return fmt.Sprintf("%s#%d", c.upstream.String(), c.consumed)
}

type bookmarkItem struct {
	artwork *pixiv.Artwork
	novel   *pixiv.Novel
}

func (t pixivTools) bookmarkListAllHandler(ctx context.Context, in bookmarkListAllIn) (*mcp.CallToolResult, outputs.Records, error) {
	plan, err := toolrun.ParseListPlan(in.ListIn)
	if err != nil {
		return outputs.Error(err)
	}
	userID, err := toolhost.ResolveUserID(t.app, ctx, in.UserID)
	if err != nil {
		return outputs.Error(err)
	}

	items, more, err := toolhost.CollectStreamsWith(ctx, t.app, plan, func(client *pixiv.Client) []pagination.Stream[bookmarkItem, bookmarkCursor] {
		return []pagination.Stream[bookmarkItem, bookmarkCursor]{
			artworkStream(client, userID, in.Restrict, in.Tag),
			novelStream(client, userID, in.Restrict, in.Tag),
		}
	})
	if err != nil {
		return outputs.Error(err)
	}

	recordItems := make([]record.Record, 0, len(items))
	for _, item := range items {
		var recordItem record.Record
		if item.artwork != nil {
			recordItem, err = record.RecordFromArtwork(*item.artwork)
		} else if item.novel != nil {
			recordItem, err = record.RecordFromNovel(*item.novel)
		} else {
			err = errors.New("bookmark aggregate returned an empty item")
		}
		if err != nil {
			return outputs.Error(err)
		}
		recordItems = append(recordItems, recordItem)
	}

	out := outputs.Records{
		Records:    recordItems,
		Pagination: toolhost.ListPagination(plan, in.Limit, len(recordItems), more),
	}
	return outputs.Result(out), out, nil
}

func artworkStream(client *pixiv.Client, userID int64, restrict, tag string) pagination.Stream[bookmarkItem, bookmarkCursor] {
	return pagination.Stream[bookmarkItem, bookmarkCursor]{
		Fetch: func(ctx context.Context, cursor bookmarkCursor) ([]bookmarkItem, bookmarkCursor, error) {
			page, err := client.UserArtworkBookmarks(ctx, pixiv.UserArtworkBookmarksRequest{
				UserID: userID, Restrict: pixiv.Restrict(restrict), Tag: tag, Cursor: cursor.upstream,
			})
			if err != nil {
				return nil, bookmarkCursor{}, err
			}
			items, next, err := artworkBatch(page, cursor.consumed)
			if err != nil {
				return nil, bookmarkCursor{}, err
			}
			return items, bookmarkCursor{upstream: next}, nil
		},
		Checkpoint: bookmarkCheckpoint,
	}
}

func novelStream(client *pixiv.Client, userID int64, restrict, tag string) pagination.Stream[bookmarkItem, bookmarkCursor] {
	return pagination.Stream[bookmarkItem, bookmarkCursor]{
		Fetch: func(ctx context.Context, cursor bookmarkCursor) ([]bookmarkItem, bookmarkCursor, error) {
			page, err := client.UserNovelBookmarks(ctx, pixiv.UserNovelBookmarksRequest{
				UserID: userID, Restrict: pixiv.Restrict(restrict), Tag: tag, Cursor: cursor.upstream,
			})
			if err != nil {
				return nil, bookmarkCursor{}, err
			}
			items, next, err := novelBatch(page, cursor.consumed)
			if err != nil {
				return nil, bookmarkCursor{}, err
			}
			return items, bookmarkCursor{upstream: next}, nil
		},
		Checkpoint: bookmarkCheckpoint,
	}
}

func artworkBatch(page sdk.Page[pixiv.Artwork], consumed int) ([]bookmarkItem, sdk.Cursor, error) {
	if consumed < 0 || consumed > len(page.Items) {
		return nil, sdk.Cursor{}, errors.New("artwork bookmark stream checkpoint exceeds batch")
	}
	items := make([]bookmarkItem, 0, len(page.Items)-consumed)
	for index := consumed; index < len(page.Items); index++ {
		item := page.Items[index]
		items = append(items, bookmarkItem{artwork: &item})
	}
	return items, page.Next, nil
}

func novelBatch(page sdk.Page[pixiv.Novel], consumed int) ([]bookmarkItem, sdk.Cursor, error) {
	if consumed < 0 || consumed > len(page.Items) {
		return nil, sdk.Cursor{}, errors.New("novel bookmark stream checkpoint exceeds batch")
	}
	items := make([]bookmarkItem, 0, len(page.Items)-consumed)
	for index := consumed; index < len(page.Items); index++ {
		item := page.Items[index]
		items = append(items, bookmarkItem{novel: &item})
	}
	return items, page.Next, nil
}

func bookmarkCheckpoint(cursor bookmarkCursor, position int) (bookmarkCursor, error) {
	if position <= 0 {
		return bookmarkCursor{}, errors.New("bookmark stream checkpoint position must be positive")
	}
	return bookmarkCursor{upstream: cursor.upstream, consumed: cursor.consumed + position}, nil
}
func (t pixivTools) registerBookmarkRemove(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_bookmark_remove", Description: "Remove an artwork or novel bookmark.", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: new(true), IdempotentHint: true, OpenWorldHint: new(true)},
		InputSchema: schemas.ClosedObject(map[string]any{"kind": schemas.EnumString("Target content type.", kindArtwork, kindNovel), "id": schemas.PositiveInteger("Positive artwork or novel ID.")}, []string{"kind", "id"})}, func(ctx context.Context, request *mcp.CallToolRequest, input bookmarkRemoveIn) (*mcp.CallToolResult, outputs.Mutation, error) {
		return t.bookmarkRemoveHandler(ctx, input)
	})
}

// bookmarkRemoveIn 的 kind 决定删除作品还是小说收藏；输出沿用原操作的 mutation envelope。
type bookmarkRemoveIn struct {
	Kind string `json:"kind"`
	ID   int64  `json:"id" jsonschema:"artwork ID"`
}

func (t pixivTools) bookmarkRemoveHandler(ctx context.Context, in bookmarkRemoveIn) (*mcp.CallToolResult, outputs.Mutation, error) {
	if in.Kind == kindNovel {
		out := outputs.Mutation{Action: "remove_novel_bookmark", NovelID: in.ID, Text: fmt.Sprintf("Removed bookmark from novel %d.", in.ID)}
		return outputs.RunMutation(&out, func() error {
			return toolhost.Write(t.app, ctx, func(ctx context.Context, client *pixiv.Client) error {
				return client.RemoveNovelBookmark(ctx, pixiv.RemoveNovelBookmarkRequest{NovelID: in.ID})
			})
		})
	}

	out := outputs.Mutation{Action: "remove_bookmark", IllustID: in.ID, Text: fmt.Sprintf("Removed bookmark from artwork %d.", in.ID)}
	return outputs.RunMutation(&out, func() error {
		return toolhost.Write(t.app, ctx, func(ctx context.Context, client *pixiv.Client) error {
			return client.RemoveArtworkBookmark(ctx, pixiv.RemoveArtworkBookmarkRequest{ArtworkID: in.ID})
		})
	})
}
func (t pixivTools) registerBookmarkTags(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_bookmark_tags", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "List artwork (default) or novel bookmark tags.", InputSchema: bookmarkTagsInputSchema(), OutputSchema: schemas.BookmarkTagsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input bookmarkTagsIn) (*mcp.CallToolResult, outputs.BookmarkTags, error) {
		return t.bookmarkTagsHandler(ctx, input)
	})
}

type bookmarkTagsIn struct {
	Kind     string `json:"kind,omitempty"`
	UserID   int64  `json:"user_id,omitempty" jsonschema:"optional user ID; defaults to authenticated user"`
	Restrict string `json:"restrict,omitempty" jsonschema:"public or private"`
	toolrun.ListIn
}

func (t pixivTools) bookmarkTagsHandler(ctx context.Context, in bookmarkTagsIn) (*mcp.CallToolResult, outputs.BookmarkTags, error) {
	if in.Kind != "" && in.Kind != kindArtwork && in.Kind != kindNovel {
		return outputs.BookmarkTagsError(errors.New("kind must be artwork or novel"))
	}
	userID, err := toolhost.ResolveUserID(t.app, ctx, in.UserID)
	if err != nil {
		return outputs.BookmarkTagsError(err)
	}
	plan, err := toolrun.ParseListPlan(in.ListIn)
	if err != nil {
		return outputs.BookmarkTagsError(err)
	}
	items, more, err := toolhost.CollectWith(ctx, t.app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.BookmarkTag, sdk.Cursor, error) {
		if in.Kind == kindNovel {
			result, err := client.UserNovelBookmarkTags(ctx, pixiv.UserNovelBookmarkTagsRequest{UserID: userID, Restrict: pixiv.Restrict(in.Restrict), Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			return result.Items, result.Next, nil
		}
		result, err := client.UserArtworkBookmarkTags(ctx, pixiv.UserArtworkBookmarkTagsRequest{UserID: userID, Restrict: pixiv.Restrict(in.Restrict), Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	})
	if err != nil {
		return outputs.BookmarkTagsError(err)
	}
	if items == nil {
		items = []pixiv.BookmarkTag{}
	}
	out := outputs.BookmarkTags{Tags: items, Pagination: toolhost.ListPagination(plan, in.Limit, len(items), more)}
	return outputs.BookmarkTagsResult(out), out, nil
}

func bookmarkTagsInputSchema() map[string]any {
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"kind":     schemas.EnumString("Content kind; defaults to artwork.", kindArtwork, kindNovel),
			"user_id":  map[string]any{"type": "integer", "description": "optional user ID; defaults to authenticated user"},
			"restrict": map[string]any{"type": "string", "description": "public or private"},
			"page":     map[string]any{"type": []string{"null", "integer"}, "description": "1-based logical page; requires a positive limit"},
			"limit":    map[string]any{"type": []string{"null", "integer"}, "description": "maximum items; 0 returns all items"},
		},
	}
}

// registerBookmarkTagsAll 注册 bookmark_tags_all，返回带内容类型的合并标签；
// bookmark_tags(kind=artwork/novel) 的输出契约保持不变。
func (t pixivTools) registerBookmarkTagsAll(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{
		Name: "pixiv_bookmark_tags_all", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)},
		Description: "List artwork bookmark tags followed by novel bookmark tags without merging names.",
		InputSchema: schemas.List(map[string]any{
			"user_id":  schemas.PositiveInteger("Optional positive Pixiv user ID; defaults to the authenticated user."),
			"restrict": schemas.EnumString("Bookmark visibility.", string(pixiv.RestrictPublic), string(pixiv.RestrictPrivate)),
		}),
		OutputSchema: schemas.BookmarkTagsAllOutputSchema(),
	}, func(ctx context.Context, request *mcp.CallToolRequest, input bookmarkTagsAllIn) (*mcp.CallToolResult, outputs.BookmarkTagsAll, error) {
		return t.bookmarkTagsAllHandler(ctx, input)
	})
}

// bookmarkTagsAllIn 描述 bookmark_tags_all 的封闭输入，分页字段与共享列表一致。
type bookmarkTagsAllIn struct {
	UserID   int64  `json:"user_id,omitempty"`
	Restrict string `json:"restrict,omitempty"`
	toolrun.ListIn
}

type bookmarkTagsCursor struct {
	upstream sdk.Cursor
	consumed int
}

// IsZero 报告是否既没有消费批内条目，也没有上游 cursor。
func (c bookmarkTagsCursor) IsZero() bool { return c.consumed == 0 && c.upstream.IsZero() }

// String 把上游 cursor 与已消费条数序列化成稳定的 cursor 文本；零值返回空串。
func (c bookmarkTagsCursor) String() string {
	if c.IsZero() {
		return ""
	}
	return fmt.Sprintf("%s#%d", c.upstream.String(), c.consumed)
}

type bookmarkTagItem struct {
	tag         pixiv.BookmarkTag
	contentType string
}

func artworkTagStream(client *pixiv.Client, userID int64, restrict string) pagination.Stream[bookmarkTagItem, bookmarkTagsCursor] {
	return pagination.Stream[bookmarkTagItem, bookmarkTagsCursor]{
		Fetch: func(ctx context.Context, cursor bookmarkTagsCursor) ([]bookmarkTagItem, bookmarkTagsCursor, error) {
			page, err := client.UserArtworkBookmarkTags(ctx, pixiv.UserArtworkBookmarkTagsRequest{
				UserID: userID, Restrict: pixiv.Restrict(restrict), Cursor: cursor.upstream,
			})
			if err != nil {
				return nil, bookmarkTagsCursor{}, err
			}
			items, next, err := tagBatch(page, cursor.consumed, kindArtwork)
			if err != nil {
				return nil, bookmarkTagsCursor{}, err
			}
			return items, bookmarkTagsCursor{upstream: next}, nil
		},
		Checkpoint: bookmarkTagsCheckpoint,
	}
}

func novelTagStream(client *pixiv.Client, userID int64, restrict string) pagination.Stream[bookmarkTagItem, bookmarkTagsCursor] {
	return pagination.Stream[bookmarkTagItem, bookmarkTagsCursor]{
		Fetch: func(ctx context.Context, cursor bookmarkTagsCursor) ([]bookmarkTagItem, bookmarkTagsCursor, error) {
			page, err := client.UserNovelBookmarkTags(ctx, pixiv.UserNovelBookmarkTagsRequest{
				UserID: userID, Restrict: pixiv.Restrict(restrict), Cursor: cursor.upstream,
			})
			if err != nil {
				return nil, bookmarkTagsCursor{}, err
			}
			items, next, err := tagBatch(page, cursor.consumed, kindNovel)
			if err != nil {
				return nil, bookmarkTagsCursor{}, err
			}
			return items, bookmarkTagsCursor{upstream: next}, nil
		},
		Checkpoint: bookmarkTagsCheckpoint,
	}
}

func bookmarkTagsCheckpoint(cursor bookmarkTagsCursor, position int) (bookmarkTagsCursor, error) {
	if position <= 0 {
		return bookmarkTagsCursor{}, errors.New("bookmark tag stream checkpoint position must be positive")
	}
	return bookmarkTagsCursor{upstream: cursor.upstream, consumed: cursor.consumed + position}, nil
}

func (t pixivTools) bookmarkTagsAllHandler(ctx context.Context, in bookmarkTagsAllIn) (*mcp.CallToolResult, outputs.BookmarkTagsAll, error) {
	plan, err := toolrun.ParseListPlan(in.ListIn)
	if err != nil {
		return outputs.BookmarkTagsAllError(err)
	}
	userID, err := toolhost.ResolveUserID(t.app, ctx, in.UserID)
	if err != nil {
		return outputs.BookmarkTagsAllError(err)
	}

	items, more, err := toolhost.CollectStreamsWith(ctx, t.app, plan, func(client *pixiv.Client) []pagination.Stream[bookmarkTagItem, bookmarkTagsCursor] {
		return []pagination.Stream[bookmarkTagItem, bookmarkTagsCursor]{
			artworkTagStream(client, userID, in.Restrict),
			novelTagStream(client, userID, in.Restrict),
		}
	})
	if err != nil {
		return outputs.BookmarkTagsAllError(err)
	}

	tags := make([]outputs.TypedBookmarkTag, 0, len(items))
	for _, item := range items {
		if item.contentType == "" {
			return outputs.BookmarkTagsAllError(errors.New("bookmark tag aggregate returned an empty content type"))
		}
		tags = append(tags, outputs.TypedBookmarkTag{
			Name: item.tag.Name, Count: item.tag.Count, ContentType: item.contentType,
		})
	}
	out := outputs.BookmarkTagsAll{
		Tags:       tags,
		Pagination: toolhost.ListPagination(plan, in.Limit, len(tags), more),
	}
	return outputs.BookmarkTagsAllResult(out), out, nil
}

func tagBatch(page sdk.Page[pixiv.BookmarkTag], consumed int, contentType string) ([]bookmarkTagItem, sdk.Cursor, error) {
	if consumed < 0 || consumed > len(page.Items) {
		return nil, sdk.Cursor{}, errors.New("bookmark tag stream checkpoint exceeds batch")
	}
	items := make([]bookmarkTagItem, 0, len(page.Items)-consumed)
	for index := consumed; index < len(page.Items); index++ {
		items = append(items, bookmarkTagItem{tag: page.Items[index], contentType: contentType})
	}
	return items, page.Next, nil
}
