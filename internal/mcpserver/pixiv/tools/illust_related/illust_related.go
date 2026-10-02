// Package illust_related 实现 pixiv_illust_related tool。
package illust_related

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/filters"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 pixiv_illust_related。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "pixiv_illust_related", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "Find artworks related to a specific illustration.", OutputSchema: records.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input relatedIn) (*mcp.CallToolResult, outputs.Records, error) {
		return handleIllustRelated(ctx, app, input)
	})
}

type relatedIn struct {
	Cursor       *string               `json:"cursor,omitempty" jsonschema:"opaque default-batch continuation; repeat original arguments and omit page and limit"`
	IllustID     int64                 `json:"illust_id"`
	IllustFilter *filters.IllustFilter `json:"illust_filter,omitempty"`
	runtime.PageLimitIn
}

func handleIllustRelated(ctx context.Context, app *runtime.App, in relatedIn) (*mcp.CallToolResult, outputs.Records, error) {
	if in.IllustID <= 0 {
		return outputs.Error(errors.New("illust_id must be a positive integer"))
	}
	plan, err := runtime.ParseListPlan(in.PageLimitIn)
	if err != nil {
		return outputs.Error(err)
	}
	var initial sdk.Cursor
	if in.Cursor != nil && (in.Page != nil || in.Limit != nil) {
		return outputs.Error(errors.New("cursor requires omitted page and limit"))
	}
	bindingInput := in
	bindingInput.Cursor = nil
	encoded, err := json.Marshal(bindingInput)
	if err != nil {
		return outputs.Error(err)
	}
	digest := sha256.Sum256(encoded)
	binding := hex.EncodeToString(digest[:])
	if in.Cursor != nil {
		cursor, parseErr := sdk.ParseCursor(*in.Cursor)
		if parseErr != nil {
			return outputs.Error(parseErr)
		}
		if err := sdk.ValidateCursor(cursor, "pixiv-mcp", "illust_related", 1, binding); err != nil {
			return outputs.Error(err)
		}
		payload, err := sdk.CursorPayload(cursor)
		if err != nil {
			return outputs.Error(err)
		}
		initial, err = sdk.ParseCursor(string(payload))
		if err != nil {
			return outputs.Error(err)
		}
	}
	ctx, err = filters.WithIllustFilter(ctx, in.IllustFilter)
	if err != nil {
		return outputs.Error(err)
	}
	fetch := func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		result, err := client.RelatedArtworks(ctx, pixiv.RelatedArtworksRequest{ArtworkID: in.IllustID, Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	var items []pixiv.Artwork
	var more bool
	var next sdk.Cursor
	if plan.OneBatch {
		items, next, err = runtime.CollectBatchWith(ctx, app, initial, fetch)
		more = !next.IsZero()
	} else {
		items, more, err = runtime.CollectWith(ctx, app, plan, fetch)
	}
	if err != nil {
		return outputs.Error(err)
	}
	recordItems, err := records.FromArtworks(items)
	if err != nil {
		return outputs.Error(err)
	}
	out := outputs.Records{Records: recordItems, Pagination: runtime.ListPagination(plan, in.Limit, len(items), more)}
	if !next.IsZero() {
		cursor, err := sdk.NewCursor("pixiv-mcp", "illust_related", 1, binding, []byte(next.String()))
		if err != nil {
			return outputs.Error(err)
		}
		out.Pagination.NextCursor = cursor.String()
	}
	return outputs.Result(out, false), out, nil
}
