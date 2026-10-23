// Package search owns the Pixiv artwork, novel and user search command routes.
package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	commands "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	pixivcmd "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivcmd"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/user"
	textutil "github.com/FlanChanXwO/pixiv-cli/internal/plaintext"
	record "github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/internal/reversesearch"
	"github.com/FlanChanXwO/pixiv-cli/internal/searchfilter"
	dateutil "github.com/FlanChanXwO/pixiv-cli/internal/searchfilter"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

const (
	searchTargetTagPartial      = "tag-partial"
	searchTargetTagExact        = "tag-exact"
	searchTargetTitleCaption    = "title-caption"
	searchTargetTagTitleCaption = "tag-title-caption"
)

type options struct {
	commands.Options
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

// ReverseSearchPort performs one reverse image search without opening Pixiv.
type ReverseSearchPort func(context.Context, ReverseSearchRequest) (reversesearch.Response, error)

type command struct {
	env           commands.Env
	runner        pixivcmd.Runner
	exec          pixivcmd.Executor
	reverseSearch ReverseSearchPort
}

// New builds the actual root `pixiv search` command.
func New(env commands.Env, exec pixivcmd.Executor, reverseSearch ReverseSearchPort) *cobra.Command {
	a := command{env: env, runner: pixivcmd.New(env.Out, exec), exec: exec, reverseSearch: reverseSearch}
	opts := options{
		searchBy:    searchTargetTagPartial,
		sortMode:    string(pixiv.SortModeDateDesc),
		typ:         "artwork",
		contentType: "all",
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
	pixivcmd.BindNDJSONFlag(cmd, &opts.ndjson)
	flags := cmd.Flags()
	flags.StringVar(&opts.searchBy, "search-by", opts.searchBy, "search field: tag-partial, tag-exact, title-caption, tag-title-caption")
	flags.StringVar(&opts.sortMode, "sort", opts.sortMode, "sort mode: date_desc, date_asc")
	flags.StringVar(&opts.period, "period", "", "time range: day, week, month, half-year, year")
	flags.StringVar(&opts.startDate, "start-date", "", "inclusive start date: YYYY-MM-DD")
	flags.StringVar(&opts.endDate, "end-date", "", "inclusive end date: YYYY-MM-DD")
	flags.StringVar(&opts.rating, "rating", "", "local rating filter: sfw, r18, r18g, mature, all")
	flags.StringVarP(&opts.typ, "type", "t", opts.typ, "entity type: artwork, novel, user")
	flags.StringVar(&opts.contentType, "content-type", opts.contentType, "artwork type filter: all, illust-and-ugoira, illust/illustration, manga, ugoira")
	flags.BoolVar(&opts.trendingTags, "trending-tags", false, "return the complete current artwork trending-tag list; WORD and list pagination are not accepted")
	flags.StringVar(&opts.resolution, "resolution", opts.resolution, "resolution filter: all, high, medium, low")
	flags.StringVar(&opts.aspectRatio, "aspect-ratio", opts.aspectRatio, "aspect ratio filter: all, landscape, portrait, square")
	flags.StringVar(&opts.drawTool, "draw-tool", "", "exact drawing tool name from the versioned drawing-tool catalog")
	flags.StringVar(&opts.aiMode, "ai-mode", opts.aiMode, "AI artwork filter: all, exclude, only")
	flags.IntVar(&opts.bookmarkMin, "bookmark-min", 0, "minimum public bookmark count; strategy controls filtering completeness")
	flags.IntVar(&opts.bookmarkMax, "bookmark-max", 0, "maximum public bookmark count; strategy controls filtering completeness")
	flags.StringVar(&opts.bookmarkStrategy, "bookmark-strategy", string(bookmarkFilterStrategyAuto), "bookmark count strategy: auto, local, best_effort, server")
	flags.StringVar(&opts.provider, "provider", "", "reverse search provider: saucenao, ascii2d-color, ascii2d-bovw, all")
	pixivcmd.BindListFlags(cmd, &opts.limit, &opts.page)
	env.BindTextValueWhen(cmd, 1, 1, 0, func(_ *cobra.Command, _ []string) bool { return !opts.trendingTags })
	commands.Bind(cmd, commands.PixivData())
	return cmd
}

func (a command) run(cmd *cobra.Command, args []string, opts options) error {
	if source := strings.Join(args, " "); isReverseSearchSource(source) {
		return a.runReverseSearch(cmd, source, opts)
	}
	if cmd.Flags().Changed("provider") {
		return a.env.UsageError(errors.New("--provider is only supported for image sources"))
	}
	if opts.trendingTags {
		if err := validateTrendingTagsFlags(cmd); err != nil {
			return err
		}
		return a.runTrendingTags(cmd, opts.Options)
	}
	entity, err := resolveEntityType(opts.typ)
	if err != nil {
		return err
	}
	if err := validateEntityFlags(cmd, entity); err != nil {
		return err
	}
	if entity == "novel" {
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
	if entity == "user" {
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
	contentType := pixiv.SearchContentType(artworkFilter.ContentType)
	aiMode, err := resolveAIMode(opts.aiMode)
	if err != nil {
		return err
	}
	aspectRatio, err := resolveAspectRatio(opts.aspectRatio)
	if err != nil {
		return err
	}
	resolution, err := resolveResolution(opts.resolution)
	if err != nil {
		return err
	}
	period, err := resolvePeriod(opts.period)
	if err != nil {
		return err
	}
	period, startDate, endDate, err := resolveDateRange(opts, period)
	if err != nil {
		return err
	}
	bookmarkMin, bookmarkMax, err := resolveBookmarkRange(cmd, opts)
	if err != nil {
		return err
	}
	word := strings.Join(args, " ")
	plan, err := pixivcmd.ParsePlan(cmd, opts.limit, opts.page)
	if err != nil {
		return err
	}
	clientReq, err := pixivcmd.BuildRequest(cmd, opts.Options)
	if err != nil {
		return err
	}
	jsonOverride := a.env.JSONOverride(cmd, opts.Options)
	if opts.ndjson && cmd.Flags().Changed("json") {
		return a.env.UsageError(errors.New("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !opts.ndjson {
		jsonOut, err = a.env.JSONOut(jsonOverride)
		if err != nil {
			return err
		}
	}
	ndjson := a.env.AutoNDJSON(cmd, opts.ndjson, jsonOut)
	query := pixiv.SearchArtworksRequest{
		Word: word, Target: target, Sort: pixiv.SortMode(opts.sortMode),
		Duration: period, StartDate: startDate, EndDate: endDate,
		ContentType: contentType, AIMode: aiMode, AspectRatio: aspectRatio, Resolution: resolution,
		Tool: opts.drawTool, BookmarkMin: bookmarkMin, BookmarkMax: bookmarkMax,
	}
	var include func(pixiv.Artwork) (bool, error)
	if artworkFilter.Rating != searchfilter.RatingAll {
		// rating 只有 DTO 中的 x_restrict 可验证；它必须绑定到 SDK cursor，
		// 但不能伪造为 upstream 的 rating/x_restrict 请求参数。
		query.CursorContext = artworkFilter.CursorContext()
		include = func(item pixiv.Artwork) (bool, error) {
			return artworkFilter.Matches(item.XRestrict, string(item.Kind)), nil
		}
	}
	if bookmarkMin != nil || bookmarkMax != nil || cmd.Flags().Changed("bookmark-strategy") {
		if bookmarkMin == nil && bookmarkMax == nil {
			return errors.New("--bookmark-strategy requires --bookmark-min or --bookmark-max")
		}
		strategy, err := resolveBookmarkFilterStrategy(opts.bookmarkStrategy)
		if err != nil {
			return err
		}
		return a.runBookmarkFiltered(cmd, clientReq, plan, jsonOut, ndjson, word, query, strategy, include)
	}
	if include != nil {
		return a.runLocallyFiltered(cmd, clientReq, plan, jsonOut, ndjson, word, query, include)
	}
	if aiMode != pixiv.SearchAIModeOnly && plan.PagePlan().Skip > 0 {
		// AI-only 在 SDK 内本地过滤，不能把过滤后的逻辑位置当作原始 offset。
		plan, query.Offset = plan.PushDownSkip()
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
	return a.runner.RunArtworks(cmd.Context(), clientReq, plan, pixivcmd.FormatOf(jsonOut, ndjson), pixivcmd.ArtworkList{Heading: pixivcmd.Static(fmt.Sprintf("illustrations for %q", word)), Fetch: fetch, Print: func(items []pixiv.Artwork, start int) error { return pixivcmd.PrintArtworksSearch(a.env.Out, items) }})
}

type artworkSearchOut struct {
	Illusts []pixiv.Artwork     `json:"illusts"`
	Filter  *bookmarkFilterJSON `json:"filter,omitempty"`
}

type bookmarkFilterJSON struct {
	Min          *int                       `json:"min,omitempty"`
	Max          *int                       `json:"max,omitempty"`
	Membership   bookmarkMembership         `json:"membership"`
	Strategy     bookmarkFilterStrategy     `json:"strategy"`
	Completeness bookmarkFilterCompleteness `json:"completeness"`
}

// runBookmarkFiltered 在 CLI adapter 内完成 bookmark 区间过滤；输出形态与
// 过滤元数据仍由 CLI 保持。
func (a command) runBookmarkFiltered(cmd *cobra.Command, account pixivcmd.Request, plan pixivcmd.Plan, jsonOut, ndjson bool, word string, query pixiv.SearchArtworksRequest, strategy bookmarkFilterStrategy, include func(pixiv.Artwork) (bool, error)) error {
	outcome, err := searchArtworks(cmd.Context(), a.runner.Execute(account), artworkSearchRequest{
		Query:      query,
		Plan:       plan.PagePlan(),
		Membership: bookmarkMembershipUnknown,
		Strategy:   strategy,
		Include:    include,
	})
	if err != nil {
		return err
	}
	return a.writeArtworkResults(jsonOut, ndjson, word, outcome.Page.Items, outcome.Filter)
}

func (a command) runLocallyFiltered(cmd *cobra.Command, account pixivcmd.Request, plan pixivcmd.Plan, jsonOut, ndjson bool, word string, query pixiv.SearchArtworksRequest, include func(pixiv.Artwork) (bool, error)) error {
	outcome, err := searchArtworks(cmd.Context(), a.runner.Execute(account), artworkSearchRequest{
		Query:   query,
		Plan:    plan.PagePlan(),
		Include: include,
	})
	if err != nil {
		return err
	}
	return a.writeArtworkResults(jsonOut, ndjson, word, outcome.Page.Items, nil)
}

func (a command) writeArtworkResults(jsonOut, ndjson bool, word string, items []pixiv.Artwork, bookmark *bookmarkFilterOutcome) error {
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
		return a.env.PrintJSON(artworkSearchOut{Illusts: artworkDTOs(items), Filter: filter})
	}
	if _, err := fmt.Fprintf(a.env.Out, "illustrations for %q\n", word); err != nil {
		return err
	}
	return pixivcmd.PrintArtworksSearch(a.env.Out, items)
}

func (a command) runTrendingTags(cmd *cobra.Command, options commands.Options) error {
	request, err := pixivcmd.BuildRequest(cmd, options)
	if err != nil {
		return err
	}
	jsonOverride := a.env.JSONOverride(cmd, options)
	jsonOut, err := a.env.JSONOut(jsonOverride)
	if err != nil {
		return err
	}
	tags, err := pixivcmd.Read(a.runner, cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) ([]pixiv.TrendingTag, error) {
		return client.TrendingArtworkTags(ctx, pixiv.TrendingArtworkTagsRequest{})
	})
	if err != nil {
		return err
	}
	if jsonOut {
		dtos := make([]pixiv.TrendingTag, 0, len(tags))
		for _, tag := range tags {
			dtos = append(dtos, tag)
		}
		return a.env.PrintJSON(struct {
			Tags []pixiv.TrendingTag `json:"tags"`
		}{Tags: dtos})
	}
	for _, tag := range tags {
		translated := tag.TranslatedName
		if translated == "" {
			translated = "none"
		}
		if _, err := fmt.Fprintf(a.env.Out, "%s (translation: %s)\n", textutil.SafeLine(tag.Tag), textutil.SafeLine(translated)); err != nil {
			return err
		}
	}
	return nil
}

func artworkDTOs(items []pixiv.Artwork) []pixiv.Artwork {
	out := make([]pixiv.Artwork, 0, len(items))
	for _, item := range items {
		out = append(out, item)
	}
	return out
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
	case "artwork", "novel", "user":
		return value, nil
	default:
		return "", errors.New("type must be one of artwork, novel, user")
	}
}

func validateEntityFlags(cmd *cobra.Command, entity string) error {
	if entity == "artwork" {
		return nil
	}
	for _, name := range []string{"content-type", "resolution", "aspect-ratio", "draw-tool", "ai-mode", "bookmark-min", "bookmark-max", "bookmark-strategy"} {
		if cmd.Flags().Changed(name) {
			return fmt.Errorf("--%s is only supported when --type artwork", name)
		}
	}
	if entity == "novel" {
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
	switch value {
	case "all", "exclude", "only":
		return pixiv.SearchAIMode(value), nil
	default:
		return "", errors.New("ai-mode must be one of all, exclude, only")
	}
}

func resolveAspectRatio(value string) (pixiv.SearchAspectRatio, error) {
	switch value {
	case "all", "landscape", "portrait", "square":
		return pixiv.SearchAspectRatio(value), nil
	default:
		return "", errors.New("aspect-ratio must be one of all, landscape, portrait, square")
	}
}

func resolveResolution(value string) (pixiv.SearchResolution, error) {
	switch value {
	case "all", "high", "medium", "low":
		return pixiv.SearchResolution(value), nil
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

func resolveBookmarkFilterStrategy(value string) (bookmarkFilterStrategy, error) {
	strategy := bookmarkFilterStrategy(value)
	switch strategy {
	case bookmarkFilterStrategyAuto, bookmarkFilterStrategyLocal, bookmarkFilterStrategyBestEffort, bookmarkFilterStrategyServer:
		return strategy, nil
	default:
		return "", errors.New("bookmark-strategy must be one of auto, local, best_effort, server")
	}
}

func resolvePeriod(value string) (pixiv.DurationFilter, error) {
	switch value {
	case "":
		return "", nil
	case "day":
		return pixiv.DurationFilter("within_last_day"), nil
	case "week":
		return pixiv.DurationFilter("within_last_week"), nil
	case "month":
		return pixiv.DurationFilter("within_last_month"), nil
	case "half-year":
		return pixiv.DurationFilter("within_half_year"), nil
	case "year":
		return pixiv.DurationFilter("within_year"), nil
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
		start = dateutil.AddMonthsClamped(today, -6)
	case "within_year":
		start = dateutil.AddMonthsClamped(today, -12)
	default:
		return "", "", false
	}
	return start.Format("2006-01-02"), today.Format("2006-01-02"), true
}

func validDate(value string) bool {
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}
