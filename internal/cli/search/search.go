// Package search 提供 pixiv search、pixiv novel search 与反向搜图命令。
//
// 包的组成：[New] 组装作品搜索，[NewNovel] 组装小说搜索，[ReverseSearchPort] 接入反向搜图；
// 收藏筛选、小说和行式输出分别在 bookmark.go、novel.go、presentation.go。
package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/user"
	"github.com/FlanChanXwO/pixiv-cli/internal/display"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/searchfilter"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/target"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/internal/reversesearch"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

const (
	searchTargetTagPartial      = "tag-partial"
	searchTargetTagExact        = "tag-exact"
	searchTargetTitleCaption    = "title-caption"
	searchTargetTagTitleCaption = "tag-title-caption"
)

type options struct {
	environment.Options
	ndjson           bool
	limit            int
	page             int
	searchBy         string
	sortMode         string
	period           string
	startDate        string
	endDate          string
	resolution       string
	aspectRatio      string
	drawTool         string
	aiMode           string
	rating           string
	typ              string
	contentType      string
	trendingTags     bool
	bookmarkStrategy string
	bookmarkMin      int
	bookmarkMax      int
	provider         string
}

// ReverseSearchPort 执行一次反向搜图；这条路径不打开 Pixiv 客户端。
type ReverseSearchPort func(context.Context, ReverseSearchRequest) (reversesearch.Response, error)

type commandRunner struct {
	env           environment.Env
	runner        session.Runner
	exec          session.Executor
	reverseSearch ReverseSearchPort
}

// New 构造根级 `pixiv search` 命令，覆盖作品、小说、用户和反向搜图入口。
func New(env environment.Env, exec session.Executor, reverseSearch ReverseSearchPort) *cobra.Command {
	a := commandRunner{env: env, runner: session.NewRunner(env.Out, exec), exec: exec, reverseSearch: reverseSearch}
	opts := options{
		searchBy:    searchTargetTagPartial,
		sortMode:    string(pixiv.SortModeDateDesc),
		typ:         string(target.NamespaceArtwork),
		contentType: string(pixiv.SearchContentTypeAll),
		resolution:  string(pixiv.SearchResolutionAll),
		aspectRatio: string(pixiv.SearchAspectRatioAll),
		aiMode:      string(pixiv.SearchAIModeAll),
	}
	cmd := &cobra.Command{
		Use:     "search [WORD]",
		Short:   "Search artworks, novels, or users",
		Example: "pixiv search \"miku\" --json",
		Args: func(cmd *cobra.Command, args []string) error {
			if opts.trendingTags {
				return env.ExactArgs(0, "pixiv search --trending-tags [options]")(cmd, args)
			}
			return env.MinArgs(1, "pixiv search [options] WORD")(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.run(cmd, args, opts)
		},
	}
	env.BindCommonFlags(cmd, &opts.Options)
	environment.BindNDJSONFlag(cmd, &opts.ndjson)
	flags := cmd.Flags()
	flags.StringVar(&opts.searchBy, "search-by", opts.searchBy, "search field: tag-partial, tag-exact, title-caption, tag-title-caption")
	flags.StringVar(&opts.sortMode, "sort", opts.sortMode, "sort mode: date_desc, date_asc, popular_desc")
	flags.StringVar(&opts.period, "period", "", "time range: day, week, month, half-year, year")
	flags.StringVar(&opts.startDate, "start-date", "", "inclusive start date: YYYY-MM-DD")
	flags.StringVar(&opts.endDate, "end-date", "", "inclusive end date: YYYY-MM-DD")
	flags.StringVar(&opts.rating, "rating", "", "local rating filter: sfw, r18, r18g, mature, all")
	flags.StringVarP(&opts.typ, "type", "t", opts.typ, "entity type: artwork, novel, user")
	flags.StringVar(&opts.contentType, "content-type", opts.contentType, "artwork type filter: all, illust-and-ugoira, illust, manga, ugoira")
	flags.BoolVar(&opts.trendingTags, "trending-tags", false, "return the complete current artwork trending-tag list; WORD and list pagination are not accepted")
	flags.StringVar(&opts.resolution, "resolution", opts.resolution, "resolution filter: all, high, medium, low")
	flags.StringVar(&opts.aspectRatio, "aspect-ratio", opts.aspectRatio, "aspect ratio filter: all, landscape, portrait, square")
	flags.StringVar(&opts.drawTool, "draw-tool", "", "exact drawing tool name from the versioned drawing-tool catalog")
	flags.StringVar(&opts.aiMode, "ai-mode", opts.aiMode, "AI artwork filter: all, exclude, only")
	flags.IntVar(&opts.bookmarkMin, "bookmark-min", 0, "minimum public bookmark count; strategy controls filtering completeness")
	flags.IntVar(&opts.bookmarkMax, "bookmark-max", 0, "maximum public bookmark count; strategy controls filtering completeness")
	flags.StringVar(&opts.bookmarkStrategy, "bookmark-strategy", string(searchfilter.BookmarkStrategyAuto), "bookmark count strategy: auto, local, best_effort, server")
	flags.StringVar(&opts.provider, "provider", "", "reverse search provider: saucenao, ascii2d-color, ascii2d-bovw, all")
	environment.BindListFlags(cmd, &opts.limit, &opts.page)
	env.BindTextValueWhen(cmd, 1, 1, 0, func(_ *cobra.Command, _ []string) bool { return !opts.trendingTags })
	environment.Bind(cmd, environment.Normal())
	return cmd
}

// artworkSearchParams 收集 artwork 搜索在解析 flag 后的查询、分页和输出参数。
type artworkSearchParams struct {
	request     session.ProxyRequest
	plan        pagination.PagePlan
	word        string
	period      pixiv.DurationFilter
	startDate   string
	endDate     string
	aiMode      pixiv.SearchAIMode
	aspectRatio pixiv.SearchAspectRatio
	resolution  pixiv.SearchResolution
	bookmarkMin *int
	bookmarkMax *int
	jsonOut     bool
	ndjson      bool
}

func (a commandRunner) artworkSearchParams(cmd *cobra.Command, opts options) (artworkSearchParams, error) {
	aiMode, err := resolveAIMode(opts.aiMode)
	if err != nil {
		return artworkSearchParams{}, err
	}
	aspectRatio, err := resolveAspectRatio(opts.aspectRatio)
	if err != nil {
		return artworkSearchParams{}, err
	}
	resolution, err := resolveResolution(opts.resolution)
	if err != nil {
		return artworkSearchParams{}, err
	}
	period, err := resolvePeriod(opts.period)
	if err != nil {
		return artworkSearchParams{}, err
	}
	period, startDate, endDate, err := resolveDateRange(opts, period)
	if err != nil {
		return artworkSearchParams{}, err
	}
	bookmarkMin, bookmarkMax, err := resolveBookmarkRange(cmd, opts)
	if err != nil {
		return artworkSearchParams{}, err
	}
	plan, err := environment.ParsePlan(cmd, opts.limit, opts.page)
	if err != nil {
		return artworkSearchParams{}, err
	}
	request, err := a.env.Request(cmd, opts.Options.ProxyOptions)
	if err != nil {
		return artworkSearchParams{}, err
	}
	if opts.ndjson && cmd.Flags().Changed("json") {
		return artworkSearchParams{}, a.env.WrapUsage(errors.New("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !opts.ndjson {
		jsonOut, err = a.env.ResolveJSON(a.env.JSONOverride(cmd, opts.Options))
		if err != nil {
			return artworkSearchParams{}, err
		}
	}
	return artworkSearchParams{
		request: request, plan: plan,
		period: period, startDate: startDate, endDate: endDate, aiMode: aiMode,
		aspectRatio: aspectRatio, resolution: resolution,
		bookmarkMin: bookmarkMin, bookmarkMax: bookmarkMax,
		jsonOut: jsonOut, ndjson: a.env.AutoNDJSON(cmd, opts.ndjson, jsonOut),
	}, nil
}

func (a commandRunner) run(cmd *cobra.Command, args []string, opts options) error {
	if source := strings.Join(args, " "); isReverseSearchSource(source) {
		return a.runReverseSearch(cmd, source, opts)
	}
	if cmd.Flags().Changed("provider") {
		return a.env.WrapUsage(errors.New("--provider is only supported for image sources"))
	}
	if opts.trendingTags {
		if err := validateTrendingTagsFlags(cmd); err != nil {
			return err
		}
		return a.runTrendingTags(cmd, opts.Options)
	}
	if err := validateSort(opts.sortMode); err != nil {
		return err
	}
	entity, err := resolveEntityType(opts.typ)
	if err != nil {
		return err
	}
	if err := validateEntityFlags(cmd, entity); err != nil {
		return err
	}
	if entity == string(target.NamespaceNovel) {
		return a.runNovelSearch(cmd, args, novelOptions{
			Options:  opts.Options,
			ndjson:   opts.ndjson,
			limit:    opts.limit,
			page:     opts.page,
			searchBy: opts.searchBy,
			sortMode: opts.sortMode,
			period:   opts.period,
		})
	}
	if entity == string(target.NamespaceUser) {
		return user.RunSearch(cmd, a.env, a.runner, args, user.SearchOptions{
			Options: opts.Options,
			NDJSON:  opts.ndjson,
			Limit:   opts.limit,
			Page:    opts.page,
		})
	}
	target, err := resolveSearchBy(opts.searchBy)
	if err != nil {
		return err
	}
	artworkFilter, err := searchfilter.NormalizeFilter(opts.rating, opts.contentType)
	if err != nil {
		return err
	}
	params, err := a.artworkSearchParams(cmd, opts)
	if err != nil {
		return err
	}
	params.word = strings.Join(args, " ")
	query := pixiv.SearchArtworksRequest{
		Word: params.word, Target: target, Sort: pixiv.SortMode(opts.sortMode),
		Duration: params.period, StartDate: params.startDate, EndDate: params.endDate,
		ContentType: pixiv.SearchContentType(artworkFilter.ContentType), AIMode: params.aiMode,
		AspectRatio: params.aspectRatio, Resolution: params.resolution,
		Tool: opts.drawTool, BookmarkMin: params.bookmarkMin, BookmarkMax: params.bookmarkMax,
	}
	var include func(pixiv.Artwork) (bool, error)
	if artworkFilter.Rating != searchfilter.RatingAll {
		// rating 只有 DTO 中的 x_restrict 可验证；它必须绑定到 SDK cursor，
		// 但不能伪造为 upstream 的 rating/x_restrict 请求参数。
		query.CursorContext = artworkFilter.CursorContext()
		include = func(item pixiv.Artwork) (bool, error) {
			return artworkFilter.Matches(item.XRestrict, string(item.ContentType)), nil
		}
	}
	if params.bookmarkMin != nil || params.bookmarkMax != nil || cmd.Flags().Changed("bookmark-strategy") {
		if params.bookmarkMin == nil && params.bookmarkMax == nil {
			return errors.New("--bookmark-strategy requires --bookmark-min or --bookmark-max")
		}
		strategy, err := resolveBookmarkFilterStrategy(opts.bookmarkStrategy)
		if err != nil {
			return err
		}
		return a.runBookmarkFiltered(cmd, params.request, params.plan, params.jsonOut, params.ndjson, query, strategy, include)
	}
	if include != nil {
		return a.runLocallyFiltered(cmd, params.request, params.plan, params.jsonOut, params.ndjson, query, include)
	}
	if params.aiMode != pixiv.SearchAIModeOnly && params.plan.Skip > 0 {
		// AI-only 在 SDK 内本地过滤，不能把过滤后的逻辑位置当作原始 offset。
		params.plan, query.Offset = params.plan.PushDownSkip()
	}
	fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		request := query
		request.Cursor = cursor
		result, err := client.SearchArtworks(ctx, request)
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return record.RunArtworks(cmd.Context(), a.runner, params.request, params.plan, record.FormatOf(params.jsonOut, params.ndjson), a.env.Err, record.ArtworkList{Fetch: fetch, Print: func(items []pixiv.Artwork, start int) error { return display.PrintArtworks(a.env.Out, items) }})
}

type artworkSearchOut struct {
	record.ListOutput[record.Record]
	Filter *bookmarkFilterJSON `json:"filter,omitempty"`
}

type bookmarkFilterJSON struct {
	Min          *int                              `json:"min,omitempty"`
	Max          *int                              `json:"max,omitempty"`
	Membership   searchfilter.BookmarkMembership   `json:"membership"`
	Strategy     searchfilter.BookmarkStrategy     `json:"strategy"`
	Completeness searchfilter.BookmarkCompleteness `json:"completeness"`
}

// runBookmarkFiltered 在 CLI adapter 内完成 bookmark 区间过滤；输出形态与
// 过滤元数据仍由 CLI 保持。
func (a commandRunner) runBookmarkFiltered(cmd *cobra.Command, account session.ProxyRequest, plan pagination.PagePlan, jsonOut, ndjson bool, query pixiv.SearchArtworksRequest, strategy searchfilter.BookmarkStrategy, include func(pixiv.Artwork) (bool, error)) error {
	outcome, err := searchArtworks(cmd.Context(), a.runner.Execute(account), artworkSearchRequest{
		Query:      query,
		Plan:       plan,
		Membership: searchfilter.BookmarkMembershipUnknown,
		Strategy:   strategy,
		Include:    include,
	})
	if err != nil {
		return err
	}
	return a.writeArtworkResults(jsonOut, ndjson, plan, outcome.Page, outcome.Filter)
}

func (a commandRunner) runLocallyFiltered(cmd *cobra.Command, account session.ProxyRequest, plan pagination.PagePlan, jsonOut, ndjson bool, query pixiv.SearchArtworksRequest, include func(pixiv.Artwork) (bool, error)) error {
	outcome, err := searchArtworks(cmd.Context(), a.runner.Execute(account), artworkSearchRequest{
		Query:   query,
		Plan:    plan,
		Include: include,
	})
	if err != nil {
		return err
	}
	return a.writeArtworkResults(jsonOut, ndjson, plan, outcome.Page, nil)
}

func (a commandRunner) writeArtworkResults(jsonOut, ndjson bool, plan pagination.PagePlan, page sdk.Page[pixiv.Artwork], bookmark *searchfilter.BookmarkOutcome) error {
	items := page.Items
	if ndjson {
		encoder := json.NewEncoder(a.env.Out)
		for _, item := range items {
			record, err := record.RecordFromArtwork(item)
			if err != nil {
				return err
			}
			if err := encoder.Encode(record); err != nil {
				return err
			}
		}
		return nil
	}
	if jsonOut {
		var filter *bookmarkFilterJSON
		if bookmark != nil {
			filter = &bookmarkFilterJSON{
				Min: bookmark.Min, Max: bookmark.Max,
				Membership: bookmark.Membership, Strategy: bookmark.Strategy,
				Completeness: bookmark.Completeness,
			}
		}
		records, err := record.FromArtworks(items)
		if err != nil {
			return err
		}
		return record.WriteJSONValue(a.env.Out, artworkSearchOut{ListOutput: record.NewListOutput(records, plan, pagination.PageResult{Returned: len(records), HasMore: !page.Next.IsZero()}), Filter: filter})
	}
	if len(items) == 0 {
		_, err := fmt.Fprintln(a.env.Err, "No results.")
		return err
	}
	return display.PrintArtworks(a.env.Out, items)
}

func (a commandRunner) runTrendingTags(cmd *cobra.Command, options environment.Options) error {
	request, err := a.env.Request(cmd, options.ProxyOptions)
	if err != nil {
		return err
	}
	jsonOverride := a.env.JSONOverride(cmd, options)
	jsonOut, err := a.env.ResolveJSON(jsonOverride)
	if err != nil {
		return err
	}
	tags, err := session.Read(a.runner, cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) ([]pixiv.TrendingTag, error) {
		return client.TrendingArtworkTags(ctx, pixiv.TrendingArtworkTagsRequest{})
	})
	if err != nil {
		return err
	}
	if jsonOut {
		return record.WriteJSONValue(a.env.Out, record.NewListOutput(tags, pagination.PagePlan{Page: 1, OneBatch: true}, pagination.PageResult{Returned: len(tags)}))
	}
	for _, tag := range tags {
		line := display.SafeLine(tag.Tag)
		if tag.TranslatedName != "" {
			line += " (" + display.SafeLine(tag.TranslatedName) + ")"
		}
		if _, err := fmt.Fprintln(a.env.Out, line); err != nil {
			return err
		}
	}
	return nil
}

func resolveSearchBy(value string) (pixiv.SearchTarget, error) {
	switch value {
	case searchTargetTagPartial:
		return pixiv.SearchTargetPartialMatchForTags, nil
	case searchTargetTagExact:
		return pixiv.SearchTargetExactMatchForTags, nil
	case searchTargetTitleCaption:
		return pixiv.SearchTargetTitleAndCaption, nil
	case searchTargetTagTitleCaption:
		return pixiv.SearchTargetKeyword, nil
	default:
		return "", fmt.Errorf("search-by must be one of %s, %s, %s, %s", searchTargetTagPartial, searchTargetTagExact, searchTargetTitleCaption, searchTargetTagTitleCaption)
	}
}

func resolveEntityType(value string) (string, error) {
	switch value {
	case string(target.NamespaceArtwork), string(target.NamespaceNovel), string(target.NamespaceUser):
		return value, nil
	default:
		return "", errors.New("type must be one of artwork, novel, user")
	}
}

func validateEntityFlags(cmd *cobra.Command, entity string) error {
	if entity == string(target.NamespaceArtwork) {
		return nil
	}
	for _, name := range []string{"content-type", "resolution", "aspect-ratio", "draw-tool", "ai-mode", "bookmark-min", "bookmark-max", "bookmark-strategy"} {
		if cmd.Flags().Changed(name) {
			return fmt.Errorf("--%s is only supported when --type artwork", name)
		}
	}
	if entity == string(target.NamespaceNovel) {
		for _, name := range []string{"start-date", "end-date", "rating"} {
			if cmd.Flags().Changed(name) {
				return fmt.Errorf("--%s is not supported when --type novel", name)
			}
		}
		return nil
	}
	for _, name := range []string{"search-by", "sort", "period", "start-date", "end-date", "rating"} {
		if cmd.Flags().Changed(name) {
			return fmt.Errorf("--%s is only supported when --type artwork or novel", name)
		}
	}
	return nil
}

func validateTrendingTagsFlags(cmd *cobra.Command) error {
	if cmd.Flags().Changed("type") || cmd.Flags().Changed("content-type") {
		return errors.New("--trending-tags cannot be combined with --type or --content-type")
	}
	for _, name := range []string{"search-by", "sort", "period", "start-date", "end-date", "rating", "resolution", "aspect-ratio", "draw-tool", "ai-mode", "bookmark-min", "bookmark-max", "bookmark-strategy", "limit", "page", "ndjson"} {
		if cmd.Flags().Changed(name) {
			return fmt.Errorf("--%s is not supported with --trending-tags", name)
		}
	}
	return nil
}

func resolveAIMode(value string) (pixiv.SearchAIMode, error) {
	mode := pixiv.SearchAIMode(value)
	switch mode {
	case pixiv.SearchAIModeAll, pixiv.SearchAIModeExclude, pixiv.SearchAIModeOnly:
		return mode, nil
	default:
		return "", errors.New("ai-mode must be one of all, exclude, only")
	}
}

func resolveAspectRatio(value string) (pixiv.SearchAspectRatio, error) {
	ratio := pixiv.SearchAspectRatio(value)
	switch ratio {
	case pixiv.SearchAspectRatioAll, pixiv.SearchAspectRatioLandscape, pixiv.SearchAspectRatioPortrait, pixiv.SearchAspectRatioSquare:
		return ratio, nil
	default:
		return "", errors.New("aspect-ratio must be one of all, landscape, portrait, square")
	}
}

func resolveResolution(value string) (pixiv.SearchResolution, error) {
	resolution := pixiv.SearchResolution(value)
	switch resolution {
	case pixiv.SearchResolutionAll, pixiv.SearchResolutionHigh, pixiv.SearchResolutionMedium, pixiv.SearchResolutionLow:
		return resolution, nil
	default:
		return "", errors.New("resolution must be one of all, high, medium, low")
	}
}

func resolveBookmarkRange(cmd *cobra.Command, opts options) (*int, *int, error) {
	var bookmarkMin, bookmarkMax *int
	if cmd.Flags().Changed("bookmark-min") {
		if opts.bookmarkMin < 0 {
			return nil, nil, errors.New("bookmark-min must be greater than or equal to zero")
		}
		minimum := opts.bookmarkMin
		bookmarkMin = &minimum
	}
	if cmd.Flags().Changed("bookmark-max") {
		if opts.bookmarkMax < 0 {
			return nil, nil, errors.New("bookmark-max must be greater than or equal to zero")
		}
		maximum := opts.bookmarkMax
		bookmarkMax = &maximum
	}
	if bookmarkMin != nil && bookmarkMax != nil && *bookmarkMin > *bookmarkMax {
		return nil, nil, errors.New("bookmark-min cannot be greater than bookmark-max")
	}
	return bookmarkMin, bookmarkMax, nil
}

// resolveBookmarkFilterStrategy 只校验 flag 取值。共享解析器要在知道是否过滤时间范围、
// 是否存在成员证据之后，才决定拒绝哪些服务端策略。
func resolveBookmarkFilterStrategy(value string) (searchfilter.BookmarkStrategy, error) {
	switch searchfilter.BookmarkStrategy(value) {
	case searchfilter.BookmarkStrategyAuto, searchfilter.BookmarkStrategyLocal, searchfilter.BookmarkStrategyBestEffort, searchfilter.BookmarkStrategyServer:
		return searchfilter.BookmarkStrategy(value), nil
	default:
		return "", errors.New("bookmark-strategy must be one of auto, local, best_effort, server")
	}
}

// 已发布的 --period 取值。半年和一年展开为日期范围，因此没有对应的
// pixiv.DurationFilter 值。
const (
	periodDay      = "day"
	periodWeek     = "week"
	periodMonth    = "month"
	periodHalfYear = "half-year"
	periodYear     = "year"
)

func resolvePeriod(value string) (pixiv.DurationFilter, error) {
	switch value {
	case "":
		return "", nil
	case periodDay:
		return pixiv.DurationLastDay, nil
	case periodWeek:
		return pixiv.DurationLastWeek, nil
	case periodMonth:
		return pixiv.DurationLastMonth, nil
	case periodHalfYear:
		return "within_half_year", nil
	case periodYear:
		return "within_year", nil
	default:
		return "", errors.New("period must be one of day, week, month, half-year, year")
	}
}

func resolveDateRange(opts options, period pixiv.DurationFilter) (pixiv.DurationFilter, string, string, error) {
	startDate := strings.TrimSpace(opts.startDate)
	endDate := strings.TrimSpace(opts.endDate)
	if period != "" && (startDate != "" || endDate != "") {
		return "", "", "", errors.New("period cannot be combined with start-date or end-date")
	}
	if startDate != "" && !validDate(startDate) || endDate != "" && !validDate(endDate) {
		return "", "", "", errors.New("start-date and end-date must use YYYY-MM-DD")
	}
	if startDate != "" && endDate != "" && startDate > endDate {
		return "", "", "", errors.New("start-date cannot be later than end-date")
	}
	if startDate, endDate, ok := quickDateRange(string(period), time.Now()); ok {
		return "", startDate, endDate, nil
	}
	return period, startDate, endDate, nil
}

const tokyoOffsetSeconds = 9 * 60 * 60

// quickDateRange 将 Pixiv 的长日期快捷项转换为包含边界的日本日期。
// App 搜索接口对半年和一年使用 start_date/end_date，而不是可靠的 duration 枚举。
func quickDateRange(value string, now time.Time) (startDate, endDate string, ok bool) {
	today := now.In(time.FixedZone("Asia/Tokyo", tokyoOffsetSeconds))
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())

	var start time.Time
	switch value {
	case "within_half_year":
		start = searchfilter.AddMonthsClamped(today, -6)
	case "within_year":
		start = searchfilter.AddMonthsClamped(today, -12)
	default:
		return "", "", false
	}
	return start.Format("2006-01-02"), today.Format("2006-01-02"), true
}

func validDate(value string) bool {
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}

func validateSort(value string) error {
	switch pixiv.SortMode(value) {
	case "", pixiv.SortModeDateDesc, pixiv.SortModeDateAsc, pixiv.SortModePopularDesc:
		return nil
	default:
		return errors.New("sort must be one of: date_desc, date_asc, popular_desc; see pixiv search --help")
	}
}
