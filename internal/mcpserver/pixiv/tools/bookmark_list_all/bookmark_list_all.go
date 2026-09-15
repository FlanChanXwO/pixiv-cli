// Package bookmark_list_all 实现 additive bookmark_list_all tool。
package bookmark_list_all

import (
	"context"
	"errors"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/pagination"
	record "github.com/FlanChanXwO/pixiv-cli/internal/shared/record"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 additive bookmark_list_all operation，不改变
// user_bookmarks 或 user_novel_bookmarks 的 wire contract。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{
		Name:        "bookmark_list_all",
		Description: "Browse artwork bookmarks followed by novel bookmarks with one logical budget.",
		InputSchema: schemas.List(map[string]any{
			"user_id":  schemas.PositiveInteger("Optional positive Pixiv user ID; defaults to the authenticated user."),
			"restrict": schemas.EnumString("Bookmark visibility.", "public", "private"),
			"tag":      map[string]any{"type": "string", "description": "Optional bookmark tag."},
		}),
		OutputSchema: records.RecordsOutputSchema(),
	}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Records, error) {
		return handle(ctx, app, input)
	})
}

// In 是 bookmark_list_all 的 closed additive input contract。
type In struct {
	UserID   int64  `json:"user_id,omitempty"`
	Restrict string `json:"restrict,omitempty"`
	Tag      string `json:"tag,omitempty"`
	runtime.PageLimitIn
}

type bookmarkCursor struct {
	upstream sdk.Cursor
	consumed int
}

func (c bookmarkCursor) IsZero() bool { return c.consumed == 0 && c.upstream.IsZero() }

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

func handle(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.Records, error) {
	plan, err := runtime.ParseListPlan(in.PageLimitIn)
	if err != nil {
		return outputs.Error(err)
	}
	userID, err := runtime.ResolveUserID(app, ctx, in.UserID)
	if err != nil {
		return outputs.Error(err)
	}

	items, more, err := runtime.CollectStreamsWith(ctx, app, plan, func(client *pixiv.Client) []pagination.Stream[bookmarkItem, bookmarkCursor] {
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
			recordItem, err = records.FromArtwork(*item.artwork)
		} else if item.novel != nil {
			recordItem, err = records.FromNovel(*item.novel)
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
		Pagination: runtime.ListPagination(plan, in.Limit, len(recordItems), more),
	}
	return outputs.Result(out, false), out, nil
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
