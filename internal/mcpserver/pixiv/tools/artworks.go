package tools

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/filters"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/schemas"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/toolhost"
	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/toolrun"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (t pixivTools) registerIllustDetail(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_illust_detail", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "Get detailed information from exactly one artwork ID or supported Pixiv URL.", OutputSchema: schemas.DetailOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input illustReferenceIn) (*mcp.CallToolResult, outputs.Detail, error) {
		return t.illustDetailHandler(ctx, input)
	})
}

type illustReferenceIn struct {
	IllustID int64  `json:"illust_id,omitempty" jsonschema:"artwork ID; provide exactly one of illust_id or url"`
	URL      string `json:"url,omitempty" jsonschema:"supported Pixiv artwork URL; provide exactly one of illust_id or url"`
}

func (t pixivTools) illustDetailHandler(ctx context.Context, in illustReferenceIn) (*mcp.CallToolResult, outputs.Detail, error) {
	id, err := resolveMCPArtworkReference(in)
	if err != nil {
		return outputs.DetailError(err)
	}
	result, err := t.app.Artwork(ctx, id)
	if err != nil {
		return outputs.DetailError(err)
	}
	artworkRecord, err := record.RecordFromArtwork(result)
	if err != nil {
		return outputs.DetailError(err)
	}
	out := outputs.Detail{Records: []record.Record{artworkRecord}}
	return outputs.DetailResult(out), out, nil
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
func (t pixivTools) registerIllustRanking(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_illust_ranking", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "Browse Pixiv rankings.", InputSchema: rankingInputSchema(), OutputSchema: schemas.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input rankingIn) (*mcp.CallToolResult, outputs.Records, error) {
		return t.illustRankingHandler(ctx, input)
	})
}

type rankingIn struct {
	Cursor       *string               `json:"cursor,omitempty" jsonschema:"opaque default-batch continuation; repeat original arguments and omit page and limit"`
	Mode         string                `json:"mode,omitempty"`
	Date         string                `json:"date,omitempty"`
	IllustFilter *filters.IllustFilter `json:"illust_filter,omitempty"`
	toolrun.ListIn
}

var rankingModes = []string{
	string(pixiv.RankingModeDay), string(pixiv.RankingModeDayMale), string(pixiv.RankingModeDayFemale),
	string(pixiv.RankingModeWeek), string(pixiv.RankingModeWeekOriginal), string(pixiv.RankingModeWeekRookie),
	string(pixiv.RankingModeMonth), string(pixiv.RankingModeDayManga), string(pixiv.RankingModeWeekManga),
	string(pixiv.RankingModeMonthManga), string(pixiv.RankingModeWeekRookieManga), string(pixiv.RankingModeDayR18),
	string(pixiv.RankingModeDayMaleR18), string(pixiv.RankingModeDayFemaleR18), string(pixiv.RankingModeWeekR18),
	string(pixiv.RankingModeWeekR18G),
}

func rankingInputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"mode":          map[string]any{"type": "string", "enum": rankingModes, "description": "Pixiv ranking mode; omitted defaults to day."},
			"date":          map[string]any{"type": "string", "pattern": "^[0-9]{4}-[0-9]{2}-[0-9]{2}$", "description": "Ranking date in YYYY-MM-DD."},
			"illust_filter": filters.IllustFilterSchema(),
			"cursor":        map[string]any{"type": "string", "description": "Opaque default-batch continuation; repeat original arguments and omit page and limit."},
			"page":          map[string]any{"type": "integer", "minimum": 1, "description": "1-based logical page; requires a positive limit."},
			"limit":         map[string]any{"type": "integer", "minimum": 0, "description": "Maximum logical results; 0 returns all; omitted reads one upstream batch."},
		},
	}
}

func isRankingMode(value string) bool {
	return slices.Contains(rankingModes, value)
}

func (t pixivTools) illustRankingHandler(ctx context.Context, in rankingIn) (*mcp.CallToolResult, outputs.Records, error) {
	if in.Mode == "" {
		in.Mode = string(pixiv.RankingModeDay)
	}
	if !isRankingMode(in.Mode) {
		return outputs.Error(errors.New("mode must be a supported Pixiv ranking mode"))
	}
	if in.Date != "" {
		if _, err := time.Parse("2006-01-02", in.Date); err != nil {
			return outputs.Error(errors.New("date must be a valid YYYY-MM-DD calendar date"))
		}
	}
	ctx, err := filters.WithIllustFilter(ctx, in.IllustFilter)
	if err != nil {
		return outputs.Error(err)
	}
	bindingInput := in
	bindingInput.Cursor = nil
	items, page, err := toolhost.CollectCursorWith(ctx, t.app, "illust_ranking", bindingInput, in.Cursor, in.ListIn, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		result, err := client.ArtworkRanking(ctx, pixiv.ArtworkRankingRequest{Mode: pixiv.RankingMode(in.Mode), Date: in.Date, Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	})
	if err != nil {
		return outputs.Error(err)
	}
	recordItems, err := record.FromArtworks(items)
	if err != nil {
		return outputs.Error(err)
	}
	out := outputs.Records{Records: recordItems, Pagination: page}
	return outputs.Result(out), out, nil
}
func (t pixivTools) registerIllustRecommended(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_illust_recommended", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "Get personalized artwork recommendations.", InputSchema: recommendedArtworkInputSchema(), OutputSchema: schemas.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input recommendedArtworkIn) (*mcp.CallToolResult, outputs.Records, error) {
		return t.illustRecommendedHandler(ctx, input)
	})
}

type recommendedArtworkIn struct {
	Cursor       *string               `json:"cursor,omitempty" jsonschema:"opaque default-batch continuation; repeat original arguments and omit page and limit"`
	IllustFilter *filters.IllustFilter `json:"illust_filter,omitempty"`
	toolrun.ListIn
}

func recommendedArtworkInputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"illust_filter": filters.IllustFilterSchema(),
			"cursor":        map[string]any{"type": "string", "description": "Opaque default-batch continuation; repeat original arguments and omit page and limit."},
			"page":          map[string]any{"type": "integer", "minimum": 1, "description": "1-based logical page; requires a positive limit."},
			"limit":         map[string]any{"type": "integer", "minimum": 0, "description": "Maximum logical results; 0 returns all; omitted reads one upstream batch."},
		},
	}
}

func (t pixivTools) illustRecommendedHandler(ctx context.Context, in recommendedArtworkIn) (*mcp.CallToolResult, outputs.Records, error) {
	ctx, err := filters.WithIllustFilter(ctx, in.IllustFilter)
	if err != nil {
		return outputs.Error(err)
	}
	bindingInput := in
	bindingInput.Cursor = nil
	items, page, err := toolhost.CollectCursorWith(ctx, t.app, "illust_recommended", bindingInput, in.Cursor, in.ListIn, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		result, err := client.RecommendedArtworks(ctx, pixiv.RecommendedArtworksRequest{Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	})
	if err != nil {
		return outputs.Error(err)
	}
	recordItems, err := record.FromArtworks(items)
	if err != nil {
		return outputs.Error(err)
	}
	out := outputs.Records{Records: recordItems, Pagination: page}
	return outputs.Result(out), out, nil
}
func (t pixivTools) registerIllustRelated(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_illust_related", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "Find artworks related to a specific illustration.", OutputSchema: schemas.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input relatedIn) (*mcp.CallToolResult, outputs.Records, error) {
		return t.illustRelatedHandler(ctx, input)
	})
}

type relatedIn struct {
	Cursor       *string               `json:"cursor,omitempty" jsonschema:"opaque default-batch continuation; repeat original arguments and omit page and limit"`
	IllustID     int64                 `json:"illust_id"`
	IllustFilter *filters.IllustFilter `json:"illust_filter,omitempty"`
	toolrun.ListIn
}

func (t pixivTools) illustRelatedHandler(ctx context.Context, in relatedIn) (*mcp.CallToolResult, outputs.Records, error) {
	if in.IllustID <= 0 {
		return outputs.Error(errors.New("illust_id must be a positive integer"))
	}
	ctx, err := filters.WithIllustFilter(ctx, in.IllustFilter)
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
	bindingInput := in
	bindingInput.Cursor = nil
	items, page, err := toolhost.CollectCursorWith(ctx, t.app, "illust_related", bindingInput, in.Cursor, in.ListIn, fetch)
	if err != nil {
		return outputs.Error(err)
	}
	recordItems, err := record.FromArtworks(items)
	if err != nil {
		return outputs.Error(err)
	}
	out := outputs.Records{Records: recordItems, Pagination: page}
	return outputs.Result(out), out, nil
}
func (t pixivTools) registerIllustSeries(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_illust_series", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "Browse artworks in a Pixiv series.", OutputSchema: schemas.RecordsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input illustSeriesIn) (*mcp.CallToolResult, outputs.Records, error) {
		return t.illustSeriesHandler(ctx, input)
	})
}

type illustSeriesIn struct {
	SeriesID int64 `json:"series_id" jsonschema:"positive Pixiv series ID"`
	toolrun.ListIn
}

func (t pixivTools) illustSeriesHandler(ctx context.Context, in illustSeriesIn) (*mcp.CallToolResult, outputs.Records, error) {
	if in.SeriesID <= 0 {
		return outputs.Error(errors.New("series_id must be a positive integer"))
	}
	plan, err := toolrun.ParseListPlan(in.ListIn)
	if err != nil {
		return outputs.Error(err)
	}
	items, more, err := toolhost.CollectWith(ctx, t.app, plan, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		result, err := client.ArtworkSeries(ctx, pixiv.ArtworkSeriesRequest{SeriesID: in.SeriesID, Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	})
	if err != nil {
		return outputs.Error(err)
	}
	recordItems, err := record.FromArtworks(items)
	if err != nil {
		return outputs.Error(err)
	}
	out := outputs.Records{Records: recordItems, Pagination: toolhost.ListPagination(plan, in.Limit, len(items), more)}
	return outputs.Result(out), out, nil
}
func (t pixivTools) registerRecommended(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_recommended", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "Get typed personalized recommendations through the Pixiv SDK.", InputSchema: recommendedInputSchema(), OutputSchema: schemas.RecommendedOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input recommendedIn) (*mcp.CallToolResult, outputs.Recommended, error) {
		return t.recommendedHandler(ctx, input)
	})
}

type recommendedIn struct {
	Cursor       *string               `json:"cursor,omitempty"`
	Kind         string                `json:"kind" jsonschema:"required: all, illust, manga, novel, or user"`
	IllustFilter *filters.IllustFilter `json:"illust_filter,omitempty"`
	NovelFilter  *filters.NovelFilter  `json:"novel_filter,omitempty"`
	UserFilter   *filters.UserFilter   `json:"user_filter,omitempty"`
	toolrun.ListIn
}

func recommendedInputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"kind"},
		"properties": map[string]any{
			"cursor":        map[string]any{"type": "string", "description": "Opaque default-batch continuation for a single kind; omit page and limit."},
			"kind":          map[string]any{"type": "string", "enum": []string{kindAll, string(pixiv.SearchContentTypeIllust), string(pixiv.SearchContentTypeManga), kindNovel, kindUser}},
			"illust_filter": filters.IllustFilterSchema(),
			"novel_filter":  filters.NovelFilterSchema(),
			"user_filter":   filters.UserFilterSchema(),
			"page":          map[string]any{"type": "integer", "minimum": 1, "description": "1-based logical page; requires a positive limit. Artwork recommendation filters are applied after selecting this upstream window."},
			"limit":         map[string]any{"type": "integer", "minimum": 0, "description": "Maximum logical results; 0 returns all; omitted reads one upstream batch. Artwork recommendation filters are applied after selecting this upstream window."},
		},
	}
}

func validateKindFilters(in recommendedIn) error {
	switch in.Kind {
	case kindAll:
		return nil
	case string(pixiv.SearchContentTypeIllust), string(pixiv.SearchContentTypeManga):
		if in.NovelFilter != nil {
			return errors.New("novel_filter conflicts with kind")
		}
		if in.UserFilter != nil {
			return errors.New("user_filter conflicts with kind")
		}
		if in.IllustFilter != nil && in.IllustFilter.Type != "" && in.IllustFilter.Type != in.Kind {
			return errors.New("illust_filter.type conflicts with kind")
		}
	case kindNovel:
		if in.IllustFilter != nil {
			return errors.New("illust_filter conflicts with kind")
		}
		if in.UserFilter != nil {
			return errors.New("user_filter conflicts with kind")
		}
	case kindUser:
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
	if kind != string(pixiv.SearchContentTypeIllust) && kind != string(pixiv.SearchContentTypeManga) {
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

func (t pixivTools) recommendedHandler(ctx context.Context, in recommendedIn) (*mcp.CallToolResult, outputs.Recommended, error) {
	out := outputs.NewRecommended()
	plan, err := parseRecommendedRequest(in)
	if err != nil {
		return outputs.RecommendedError(err)
	}
	ctx, err = filters.WithMixedFilters(ctx, in.IllustFilter, in.NovelFilter, in.UserFilter)
	if err != nil {
		return outputs.RecommendedError(err)
	}
	initial, err := recommendedInitialCursor(in, plan)
	if err != nil {
		return outputs.RecommendedError(err)
	}
	execute := t.app.ReadExecutor()
	if execute == nil {
		return outputs.RecommendedError(sdk.NewError("pixiv", "Recommended", sdk.LocalStateError,
			sdk.WithDetail("sdk pooled operation is not configured")))
	}
	err = execute(ctx, func(ctx context.Context, client *pixiv.Client) (bool, error) {
		// 重放必须丢弃前一账号/尝试的记录与分页状态。
		out = outputs.NewRecommended()
		if err := t.recommendedArtworkStreams(ctx, client, in, plan, initial, &out); err != nil {
			return false, err
		}
		if err := t.recommendedNovelStream(ctx, client, in, plan, initial, &out); err != nil {
			return false, err
		}
		if err := t.recommendedUserStream(ctx, client, in, plan, initial, &out); err != nil {
			return false, err
		}
		return false, nil
	})
	if err != nil {
		return outputs.RecommendedError(err)
	}
	return outputs.RecordResult(out.Records, false, ""), out, nil
}

func parseRecommendedRequest(in recommendedIn) (toolrun.ListPlan, error) {
	plan, err := toolrun.ParseListPlan(in.ListIn)
	if err == nil && in.Kind != kindAll && in.Kind != string(pixiv.SearchContentTypeIllust) && in.Kind != string(pixiv.SearchContentTypeManga) && in.Kind != kindNovel && in.Kind != kindUser {
		err = errors.New("kind must be one of: all, illust, manga, novel, user")
	}
	if err != nil {
		return toolrun.ListPlan{}, err
	}
	if err := validateKindFilters(in); err != nil {
		return toolrun.ListPlan{}, err
	}
	return plan, nil
}

func recommendedInitialCursor(in recommendedIn, plan toolrun.ListPlan) (sdk.Cursor, error) {
	if in.Cursor == nil {
		return sdk.Cursor{}, nil
	}
	if !plan.OneBatch || in.Kind == kindAll {
		return sdk.Cursor{}, errors.New("cursor requires a single kind and omitted page and limit")
	}
	binding, err := toolhost.BindCursor("recommended", streamInput(in, in.Kind))
	if err != nil {
		return sdk.Cursor{}, err
	}
	return binding.Decode(in.Cursor)
}

func (t pixivTools) recommendedArtworkStreams(ctx context.Context, client *pixiv.Client, in recommendedIn, plan toolrun.ListPlan, initial sdk.Cursor, out *outputs.Recommended) error {
	var visualItems []pixiv.Artwork
	var visualMore bool
	var visualNext sdk.Cursor
	if in.Kind == kindAll || in.Kind == string(pixiv.SearchContentTypeIllust) || in.Kind == string(pixiv.SearchContentTypeManga) {
		rawArtworkCtx, err := filters.WithIllustFilter(ctx, nil)
		if err != nil {
			return err
		}
		visualItems, visualMore, visualNext, err = toolhost.CollectPagesFrom(rawArtworkCtx, plan, initial, func(ctx context.Context, c sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
			r, e := client.RecommendedArtworks(ctx, pixiv.RecommendedArtworksRequest{Cursor: c})
			if e != nil {
				return nil, sdk.Cursor{}, e
			}
			return r.Items, r.Next, nil
		})
		if err != nil {
			return err
		}
	}
	if in.Kind == kindAll || in.Kind == string(pixiv.SearchContentTypeIllust) {
		if err := appendRecommendedArtworks(ctx, out, plan, in, string(pixiv.SearchContentTypeIllust), visualItems, visualMore, visualNext); err != nil {
			return err
		}
	}
	if in.Kind == kindAll || in.Kind == string(pixiv.SearchContentTypeManga) {
		if err := appendRecommendedArtworks(ctx, out, plan, in, string(pixiv.SearchContentTypeManga), visualItems, visualMore, visualNext); err != nil {
			return err
		}
	}
	return nil
}

func appendRecommendedArtworks(ctx context.Context, out *outputs.Recommended, plan toolrun.ListPlan, in recommendedIn, kind string, items []pixiv.Artwork, more bool, next sdk.Cursor) error {
	artworkCtx, err := filters.WithIllustFilter(ctx, artworkFilterForKind(kind, in.IllustFilter))
	if err != nil {
		return err
	}
	filtered := filters.FilterPage(artworkCtx, items, make(map[string]struct{}))
	recordItems, err := record.FromArtworks(filtered)
	if err != nil {
		return err
	}
	out.Records = append(out.Records, recordItems...)
	page := outputs.RecommendedPage(plan, in.Limit, len(filtered), more)
	page.NextCursor, err = streamContinuation(in, kind, next)
	if err != nil {
		return err
	}
	if kind == string(pixiv.SearchContentTypeIllust) {
		out.Pagination.Illust = page
	} else {
		out.Pagination.Manga = page
	}
	return nil
}

func (t pixivTools) recommendedNovelStream(ctx context.Context, client *pixiv.Client, in recommendedIn, plan toolrun.ListPlan, initial sdk.Cursor, out *outputs.Recommended) error {
	if in.Kind != kindAll && in.Kind != kindNovel {
		return nil
	}
	novelCtx, err := filters.WithNovelFilter(ctx, in.NovelFilter)
	if err != nil {
		return err
	}
	items, more, next, err := toolhost.CollectPagesFrom(novelCtx, plan, initial, func(ctx context.Context, c sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
		r, e := client.RecommendedNovels(ctx, pixiv.RecommendedNovelsRequest{Cursor: c})
		if e != nil {
			return nil, sdk.Cursor{}, e
		}
		return r.Items, r.Next, nil
	})
	if err != nil {
		return err
	}
	recordItems, err := record.FromNovels(items)
	if err != nil {
		return err
	}
	out.Records = append(out.Records, recordItems...)
	out.Pagination.Novel = outputs.RecommendedPage(plan, in.Limit, len(items), more)
	out.Pagination.Novel.NextCursor, err = streamContinuation(in, kindNovel, next)
	return err
}

func (t pixivTools) recommendedUserStream(ctx context.Context, client *pixiv.Client, in recommendedIn, plan toolrun.ListPlan, initial sdk.Cursor, out *outputs.Recommended) error {
	if in.Kind != kindAll && in.Kind != kindUser {
		return nil
	}
	userCtx, err := filters.WithUserFilter(ctx, in.UserFilter)
	if err != nil {
		return err
	}
	items, more, next, err := toolhost.CollectPagesFrom(userCtx, plan, initial, func(ctx context.Context, c sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error) {
		r, e := client.RecommendedUsers(ctx, pixiv.RecommendedUsersRequest{Cursor: c})
		if e != nil {
			return nil, sdk.Cursor{}, e
		}
		return r.Items, r.Next, nil
	})
	if err != nil {
		return err
	}
	recordItems, err := record.FromUserPreviews(items)
	if err != nil {
		return err
	}
	out.Records = append(out.Records, recordItems...)
	out.Pagination.User = outputs.RecommendedPage(plan, in.Limit, len(items), more)
	out.Pagination.User.NextCursor, err = streamContinuation(in, kindUser, next)
	return err
}

// streamInput 只携带客户端所选 kind 的续读参数。
func streamInput(in recommendedIn, kind string) recommendedIn {
	in.Kind = kind
	in.Cursor = nil
	if kind != string(pixiv.SearchContentTypeIllust) && kind != string(pixiv.SearchContentTypeManga) {
		in.IllustFilter = nil
	}
	if kind != kindNovel {
		in.NovelFilter = nil
	}
	if kind != kindUser {
		in.UserFilter = nil
	}
	return in
}

func streamContinuation(in recommendedIn, kind string, next sdk.Cursor) (string, error) {
	if next.IsZero() {
		return "", nil
	}
	if (kind == string(pixiv.SearchContentTypeIllust) || kind == string(pixiv.SearchContentTypeManga)) && in.IllustFilter != nil && in.IllustFilter.Type != "" && in.IllustFilter.Type != kind {
		return "", nil
	}
	binding, err := toolhost.BindCursor("recommended", streamInput(in, kind))
	if err != nil {
		return "", err
	}
	return binding.Encode(next)
}
func (t pixivTools) registerTrendingTagsIllust(server *mcp.Server) {
	toolhost.AddTool(t.app, server, &mcp.Tool{Name: "pixiv_trending_tags_illust", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}, Description: "Get currently trending illustration tags.", InputSchema: trendingInputSchema(), OutputSchema: schemas.TrendingTagsOutputSchema()}, func(ctx context.Context, request *mcp.CallToolRequest, input trendingTagsIllustIn) (*mcp.CallToolResult, outputs.TrendingTags, error) {
		return t.trendingTagsIllustHandler(ctx, input)
	})
}

type trendingTagsIllustIn struct{}

func trendingInputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           map[string]any{},
	}
}

func (t pixivTools) trendingTagsIllustHandler(ctx context.Context, _ trendingTagsIllustIn) (*mcp.CallToolResult, outputs.TrendingTags, error) {
	result, err := toolhost.Read(t.app, ctx, func(ctx context.Context, client *pixiv.Client) ([]pixiv.TrendingTag, error) {
		return client.TrendingArtworkTags(ctx, pixiv.TrendingArtworkTagsRequest{})
	})
	if err != nil {
		return outputs.TrendingTagsError(err)
	}
	out := outputs.TrendingTags{Tags: []pixiv.TrendingTag{}}
	if len(result) == 0 {
		out.Text = "No trending tags found."
		return outputs.TrendingTagsResult(out, false), out, nil
	}
	lines := make([]string, 0, len(result))
	for _, tag := range result {
		out.Tags = append(out.Tags, tag)
		translated := tag.TranslatedName
		if translated == "" {
			translated = "none"
		}
		lines = append(lines, fmt.Sprintf("- %s (translation: %s)", tag.Tag, translated))
	}
	out.Text = "Trending tags:\n" + strings.Join(lines, "\n")
	return outputs.TrendingTagsResult(out, false), out, nil
}
