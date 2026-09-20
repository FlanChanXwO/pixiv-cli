// Package bookmark_tags_all 实现 additive bookmark_tags_all tool。
package bookmark_tags_all

import (
	"context"
	"errors"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/pagination"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 additive bookmark_tags_all operation，不改变
// bookmark_tags 或 novel_bookmark_tags 的 output contract。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{
		Name:        "bookmark_tags_all",
		Description: "List artwork bookmark tags followed by novel bookmark tags without merging names.",
		InputSchema: schemas.List(map[string]any{
			"user_id":  schemas.PositiveInteger("Optional positive Pixiv user ID; defaults to the authenticated user."),
			"restrict": schemas.EnumString("Bookmark visibility.", "public", "private"),
		}),
		OutputSchema: records.BookmarkTagsAllOutputSchema(),
	}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.BookmarkTagsAll, error) {
		return handle(ctx, app, input)
	})
}

// In 是 bookmark_tags_all 的 closed additive input contract。
type In struct {
	UserID   int64  `json:"user_id,omitempty"`
	Restrict string `json:"restrict,omitempty"`
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

type bookmarkTagItem struct {
	tag         pixiv.BookmarkTag
	contentType string
}

func handle(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.BookmarkTagsAll, error) {
	plan, err := runtime.ParseListPlan(in.PageLimitIn)
	if err != nil {
		return outputs.BookmarkTagsAllError(err)
	}
	userID, err := runtime.ResolveUserID(app, ctx, in.UserID)
	if err != nil {
		return outputs.BookmarkTagsAllError(err)
	}

	items, more, err := runtime.CollectStreamsWith(ctx, app, plan, func(client *pixiv.Client) []pagination.Stream[bookmarkTagItem, bookmarkCursor] {
		return []pagination.Stream[bookmarkTagItem, bookmarkCursor]{
			artworkStream(client, userID, in.Restrict),
			novelStream(client, userID, in.Restrict),
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
		Pagination: runtime.ListPagination(plan, in.Limit, len(tags), more),
	}
	return outputs.BookmarkTagsAllResult(out, len(tags)), out, nil
}

func artworkStream(client *pixiv.Client, userID int64, restrict string) pagination.Stream[bookmarkTagItem, bookmarkCursor] {
	return pagination.Stream[bookmarkTagItem, bookmarkCursor]{
		Fetch: func(ctx context.Context, cursor bookmarkCursor) ([]bookmarkTagItem, bookmarkCursor, error) {
			page, err := client.UserArtworkBookmarkTags(ctx, pixiv.UserArtworkBookmarkTagsRequest{
				UserID: userID, Restrict: pixiv.Restrict(restrict), Cursor: cursor.upstream,
			})
			if err != nil {
				return nil, bookmarkCursor{}, err
			}
			items, next, err := tagBatch(page, cursor.consumed, "artwork")
			if err != nil {
				return nil, bookmarkCursor{}, err
			}
			return items, bookmarkCursor{upstream: next}, nil
		},
		Checkpoint: bookmarkCheckpoint,
	}
}

func novelStream(client *pixiv.Client, userID int64, restrict string) pagination.Stream[bookmarkTagItem, bookmarkCursor] {
	return pagination.Stream[bookmarkTagItem, bookmarkCursor]{
		Fetch: func(ctx context.Context, cursor bookmarkCursor) ([]bookmarkTagItem, bookmarkCursor, error) {
			page, err := client.UserNovelBookmarkTags(ctx, pixiv.UserNovelBookmarkTagsRequest{
				UserID: userID, Restrict: pixiv.Restrict(restrict), Cursor: cursor.upstream,
			})
			if err != nil {
				return nil, bookmarkCursor{}, err
			}
			items, next, err := tagBatch(page, cursor.consumed, "novel")
			if err != nil {
				return nil, bookmarkCursor{}, err
			}
			return items, bookmarkCursor{upstream: next}, nil
		},
		Checkpoint: bookmarkCheckpoint,
	}
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

func bookmarkCheckpoint(cursor bookmarkCursor, position int) (bookmarkCursor, error) {
	if position <= 0 {
		return bookmarkCursor{}, errors.New("bookmark tag stream checkpoint position must be positive")
	}
	return bookmarkCursor{upstream: cursor.upstream, consumed: cursor.consumed + position}, nil
}
