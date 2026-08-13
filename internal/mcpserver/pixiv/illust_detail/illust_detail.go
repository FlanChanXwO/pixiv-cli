// Package illust_detail 实现 illust_detail tool。
package illust_detail

import (
	"context"
	"errors"
	"strings"

	pipeline "github.com/FlanChanXwO/pixiv-cli/internal/cli/pipeline"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 illust_detail。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "illust_detail", Description: "Get detailed information from exactly one artwork ID or supported Pixiv URL.", OutputSchema: records.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input illustReferenceIn) (*mcp.CallToolResult, outputs.UserDetail, error) {
		return handleIllustDetail(ctx, app, input)
	})
}

type illustReferenceIn struct {
	IllustID int64  `json:"illust_id,omitempty" jsonschema:"artwork ID; provide exactly one of illust_id or url"`
	URL      string `json:"url,omitempty" jsonschema:"supported Pixiv artwork URL; provide exactly one of illust_id or url"`
}

func handleIllustDetail(ctx context.Context, app *runtime.App, in illustReferenceIn) (*mcp.CallToolResult, outputs.UserDetail, error) {
	id, err := resolveMCPArtworkReference(in)
	if err != nil {
		return outputs.UserDetailError(err)
	}
	result, err := runtime.Read(app, ctx, func(ctx context.Context, client *pixiv.Client) (pixiv.Artwork, error) {
		return client.Artwork(ctx, pixiv.ArtworkRequest{ArtworkID: id})
	})
	if err != nil {
		return outputs.UserDetailError(err)
	}
	record, err := records.FromArtwork(result)
	if err != nil {
		return outputs.UserDetailError(err)
	}
	out := outputs.UserDetail{Records: []pipeline.Record{record}}
	return outputs.UserDetailResult(out, false), out, nil
}

func resolveMCPArtworkReference(in illustReferenceIn) (int64, error) {
	hasID := in.IllustID != 0
	hasURL := strings.TrimSpace(in.URL) != ""
	if hasID == hasURL {
		return 0, errors.New("provide exactly one of illust_id or url")
	}
	if hasID {
		if in.IllustID <= 0 {
			return 0, errors.New("illust_id must be a positive integer")
		}
		return in.IllustID, nil
	}
	reference, err := pixiv.ParseURL(in.URL)
	if err != nil {
		return 0, err
	}
	if reference.Kind != pixiv.ReferenceKindArtwork {
		return 0, errors.New("URL does not name a Pixiv artwork")
	}
	return reference.ID, nil
}
