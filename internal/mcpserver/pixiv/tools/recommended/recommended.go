// Package recommended 实现 recommended tool。
package recommended

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/filters"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register 注册 recommended。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "recommended", Description: "Get typed personalized recommendations through the Pixiv SDK.", InputSchema: recommendedInputSchema(), OutputSchema: records.RecommendedOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, outputs.Recommended, error) {
		return handleRecommended(ctx, app, input)
	})
}

type In struct {
	Kind         string                `json:"kind" jsonschema:"required: all, illust, manga, novel, or user"`
	IllustFilter *filters.IllustFilter `json:"illust_filter,omitempty"`
	NovelFilter  *filters.NovelFilter  `json:"novel_filter,omitempty"`
	UserFilter   *filters.UserFilter   `json:"user_filter,omitempty"`
	runtime.PageLimitIn
}

func recommendedInputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"kind"},
		"properties": map[string]any{
			"kind":          map[string]any{"type": "string", "enum": []string{"all", "illust", "manga", "novel", "user"}},
			"illust_filter": filters.IllustFilterSchema(),
			"novel_filter":  filters.NovelFilterSchema(),
			"user_filter":   filters.UserFilterSchema(),
			"page":          map[string]any{"type": "integer", "minimum": 1, "description": "1-based logical page; requires a positive limit."},
			"limit":         map[string]any{"type": "integer", "minimum": 0, "description": "Maximum logical results; 0 returns all; omitted reads one upstream batch."},
		},
	}
}

func validateKindFilters(in In) error {
	switch in.Kind {
	case "all":
		return nil
	case "illust", "manga":
		if in.NovelFilter != nil {
			return errors.New("novel_filter conflicts with kind")
		}
		if in.UserFilter != nil {
			return errors.New("user_filter conflicts with kind")
		}
		if in.IllustFilter != nil && in.IllustFilter.Type != "" && in.IllustFilter.Type != in.Kind {
			return errors.New("illust_filter.type conflicts with kind")
		}
	case "novel":
		if in.IllustFilter != nil {
			return errors.New("illust_filter conflicts with kind")
		}
		if in.UserFilter != nil {
			return errors.New("user_filter conflicts with kind")
		}
	case "user":
		if in.IllustFilter != nil {
			return errors.New("illust_filter conflicts with kind")
		}
		if in.NovelFilter != nil {
			return errors.New("novel_filter conflicts with kind")
		}
	}
	return nil
}

func artworkFilterForKind(kind string, filter *filters.IllustFilter) *filters.IllustFilter {
	if kind != "illust" && kind != "manga" {
		return filter
	}
	if filter == nil {
		return &filters.IllustFilter{Type: kind}
	}
	copy := *filter
	if copy.Type == "" {
		copy.Type = kind
	}
	return &copy
}

func handleRecommended(ctx context.Context, app *runtime.App, in In) (*mcp.CallToolResult, outputs.Recommended, error) {
	out := outputs.NewRecommended()
	plan, err := runtime.ParseListPlan(in.PageLimitIn)
	if err == nil && in.Kind != "all" && in.Kind != "illust" && in.Kind != "manga" && in.Kind != "novel" && in.Kind != "user" {
		err = errors.New("kind must be one of: all, illust, manga, novel, user")
	}
	if err != nil {
		return outputs.RecommendedError(err)
	}
	if err := validateKindFilters(in); err != nil {
		return outputs.RecommendedError(err)
	}
	ctx, err = filters.WithMixedFilters(ctx, in.IllustFilter, in.NovelFilter, in.UserFilter)
	if err != nil {
		return outputs.RecommendedError(err)
	}
	execute := app.Execute()
	if execute == nil {
		return outputs.RecommendedError(sdk.NewError("pixiv", "Recommended", sdk.LocalStateError,
			sdk.WithDetail("sdk pooled operation is not configured")))
	}
	err = execute(ctx, func(ctx context.Context, client *pixiv.Client) (bool, error) {
		if in.Kind == "all" || in.Kind == "illust" {
			artworkCtx, filterErr := filters.WithIllustFilter(ctx, artworkFilterForKind(in.Kind, in.IllustFilter))
			if filterErr != nil {
				return false, filterErr
			}
			items, more, fetchErr := runtime.CollectPages(artworkCtx, plan, func(ctx context.Context, c sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
				r, e := client.RecommendedArtworks(ctx, pixiv.RecommendedArtworksRequest{Cursor: c})
				if e != nil {
					return nil, sdk.Cursor{}, e
				}
				return r.Items, r.Next, nil
			})
			if fetchErr != nil {
				return false, fetchErr
			}
			recordItems, mapErr := records.FromArtworks(items)
			if mapErr != nil {
				return false, mapErr
			}
			out.Records = append(out.Records, recordItems...)
			out.Pagination.Illust = outputs.RecommendedPage(plan, in.Limit, len(items), more)
		}
		if in.Kind == "all" || in.Kind == "manga" {
			artworkCtx, filterErr := filters.WithIllustFilter(ctx, artworkFilterForKind("manga", in.IllustFilter))
			if filterErr != nil {
				return false, filterErr
			}
			items, more, fetchErr := runtime.CollectPages(artworkCtx, plan, func(ctx context.Context, c sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
				r, e := client.RecommendedArtworks(ctx, pixiv.RecommendedArtworksRequest{Cursor: c})
				if e != nil {
					return nil, sdk.Cursor{}, e
				}
				return r.Items, r.Next, nil
			})
			if fetchErr != nil {
				return false, fetchErr
			}
			recordItems, mapErr := records.FromArtworks(items)
			if mapErr != nil {
				return false, mapErr
			}
			out.Records = append(out.Records, recordItems...)
			out.Pagination.Manga = outputs.RecommendedPage(plan, in.Limit, len(items), more)
		}
		if in.Kind == "all" || in.Kind == "novel" {
			novelCtx, filterErr := filters.WithNovelFilter(ctx, in.NovelFilter)
			if filterErr != nil {
				return false, filterErr
			}
			items, more, fetchErr := runtime.CollectPages(novelCtx, plan, func(ctx context.Context, c sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
				r, e := client.RecommendedNovels(ctx, pixiv.RecommendedNovelsRequest{Cursor: c})
				if e != nil {
					return nil, sdk.Cursor{}, e
				}
				return r.Items, r.Next, nil
			})
			if fetchErr != nil {
				return false, fetchErr
			}
			recordItems, mapErr := records.FromNovels(items)
			if mapErr != nil {
				return false, mapErr
			}
			out.Records = append(out.Records, recordItems...)
			out.Pagination.Novel = outputs.RecommendedPage(plan, in.Limit, len(items), more)
		}
		if in.Kind == "all" || in.Kind == "user" {
			userCtx, filterErr := filters.WithUserFilter(ctx, in.UserFilter)
			if filterErr != nil {
				return false, filterErr
			}
			items, more, fetchErr := runtime.CollectPages(userCtx, plan, func(ctx context.Context, c sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error) {
				r, e := client.RecommendedUsers(ctx, pixiv.RecommendedUsersRequest{Cursor: c})
				if e != nil {
					return nil, sdk.Cursor{}, e
				}
				return r.Items, r.Next, nil
			})
			if fetchErr != nil {
				return false, fetchErr
			}
			recordItems, mapErr := records.FromUserPreviews(items)
			if mapErr != nil {
				return false, mapErr
			}
			out.Records = append(out.Records, recordItems...)
			out.Pagination.User = outputs.RecommendedPage(plan, in.Limit, len(items), more)
		}
		return false, nil
	})
	if err != nil {
		return outputs.RecommendedError(err)
	}
	return records.Result(out.Records, false, ""), out, nil
}
