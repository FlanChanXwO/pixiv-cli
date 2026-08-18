// Package search owns the Pixiv artwork, novel and user search command routes.
package search

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	requirements "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/internal/listing"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/user"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pipeline"
	record "github.com/FlanChanXwO/pixiv-cli/internal/shared/record"
	dateutil "github.com/FlanChanXwO/pixiv-cli/internal/utils/date"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const (
	searchTargetTagPartial      = "tag-partial"
	searchTargetTagExact        = "tag-exact"
	searchTargetTitleCaption    = "title-caption"
	searchTargetTagTitleCaption = "tag-title-caption"
)

type options struct {
	CommandOptions
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
}

// Request 是 search 一次执行解析出的传输覆写值；它不持有 client 或资源。
type Request struct {
	UserID             int64
	HTTPSProxyOverride *string
}

// CommandOptions 是 search 自己声明的通用 flag 值。
type CommandOptions struct {
	Proxy   string
	NoProxy bool
	JSON    bool
}

// Dependencies 是 search、novel search 与 user-search 兼容路由的窄执行端口。
type Dependencies struct {
	Input      io.Reader
	Output     io.Writer
	UsageError func(error) error
	JSONOut    func(*bool) (bool, error)
	Pooled     func(context.Context, Request, func(context.Context, *pixiv.Client) (bool, error)) error
}

type command struct {
	data Dependencies
}

func (d Dependencies) usage(err error) error {
	if err == nil || d.UsageError == nil {
		return err
	}
	return d.UsageError(err)
}

func (d Dependencies) exactArgs(count int, usage string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) != count {
			return fmt.Errorf("usage: %s", usage)
		}
		return nil
	}
}

func (d Dependencies) minArgs(count int, usage string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) < count {
			return fmt.Errorf("usage: %s", usage)
		}
		return nil
	}
}

func (d Dependencies) bindCommonFlags(cmd *cobra.Command, opts *CommandOptions) {
	cmd.Flags().BoolVarP(&opts.JSON, "json", "j", false, "print JSON")
	cmd.Flags().StringVar(&opts.Proxy, "proxy", "", "proxy URL (http, https, socks5, or socks5h) for this command")
	cmd.Flags().BoolVar(&opts.NoProxy, "no-proxy", false, "clear the configured proxy for this command")
}

func (d Dependencies) bindTextValue(cmd *cobra.Command, minArgs, maxArgs, fillPosition int) {
	pipeline.Bind(cmd, pipeline.InputSpec{Codec: pipeline.TextValue, MinArgs: minArgs, MaxArgs: maxArgs, FillPosition: fillPosition, Reader: d.Input, UsageError: d.usage})
}

func (d Dependencies) bindTextValueWhen(cmd *cobra.Command, minArgs, maxArgs, fillPosition int, enabled func(*cobra.Command, []string) bool) {
	pipeline.Bind(cmd, pipeline.InputSpec{Codec: pipeline.TextValue, MinArgs: minArgs, MaxArgs: maxArgs, FillPosition: fillPosition, Reader: d.Input, UsageError: d.usage, Enabled: enabled})
}

func (d Dependencies) bindNoInput(cmd *cobra.Command) {
	pipeline.Bind(cmd, pipeline.InputSpec{Codec: pipeline.NoInput, MinArgs: 0, MaxArgs: 0, Reader: d.Input, UsageError: d.usage})
}

func (d Dependencies) request(cmd *cobra.Command, opts CommandOptions) (Request, error) {
	if cmd.Flags().Changed("proxy") && cmd.Flags().Changed("no-proxy") {
		return Request{}, errors.New("use either --proxy or --no-proxy, not both")
	}
	request := Request{}
	if cmd.Flags().Changed("no-proxy") && opts.NoProxy {
		empty := ""
		request.HTTPSProxyOverride = &empty
	} else if cmd.Flags().Changed("proxy") {
		request.HTTPSProxyOverride = &opts.Proxy
	}
	return request, nil
}

func (d Dependencies) jsonOverride(cmd *cobra.Command, opts CommandOptions) *bool {
	if !cmd.Flags().Changed("json") {
		return nil
	}
	value := opts.JSON
	return &value
}

func (d Dependencies) jsonOut(override *bool) (bool, error) {
	if d.JSONOut == nil {
		return false, errors.New("pixiv search JSON output resolver is not configured")
	}
	return d.JSONOut(override)
}

func (d Dependencies) shouldAutoNDJSON(cmd *cobra.Command, ndjson, jsonOut bool) bool {
	if ndjson || jsonOut || cmd.Flags().Changed("json") {
		return ndjson
	}
	file, ok := d.Output.(interface{ Fd() uintptr })
	return ok && !term.IsTerminal(int(file.Fd()))
}

func (d Dependencies) writeJSON(value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var out bytes.Buffer
	if err := json.Indent(&out, body, "", "  "); err != nil {
		return err
	}
	_, err = io.WriteString(d.Output, out.String()+"\n")
	return err
}

func read[T any](d Dependencies, ctx context.Context, request Request, invoke func(context.Context, *pixiv.Client) (T, error)) (T, error) {
	var zero T
	if d.Pooled == nil {
		return zero, errors.New("pixiv pooled operation is not configured")
	}
	var result T
	err := d.Pooled(ctx, request, func(ctx context.Context, client *pixiv.Client) (bool, error) {
		var err error
		result, err = invoke(ctx, client)
		return false, err
	})
	if err != nil {
		return zero, err
	}
	return result, nil
}

// New builds the actual root `pixiv search` command.
func New(data Dependencies) *cobra.Command {
	a := command{data: data}
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
				return data.exactArgs(0, "pixiv search --trending-tags [options]")(cmd, args)
			}
			return data.minArgs(1, "pixiv search [options] WORD")(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.run(cmd, args, opts)
		},
	}
	data.bindCommonFlags(cmd, &opts.CommandOptions)
	listing.BindNDJSONFlag(cmd, &opts.ndjson)
	flags := cmd.Flags()
	flags.StringVar(&opts.searchBy, "search-by", opts.searchBy, "search field: tag-partial, tag-exact, title-caption, tag-title-caption")
	flags.StringVar(&opts.sortMode, "sort", opts.sortMode, "sort mode: date_desc, date_asc")
	flags.StringVar(&opts.period, "period", "", "time range: day, week, month, half-year, year")
	flags.StringVar(&opts.startDate, "start-date", "", "inclusive start date: YYYY-MM-DD")
	flags.StringVar(&opts.endDate, "end-date", "", "inclusive end date: YYYY-MM-DD")
	flags.StringVar(&opts.rating, "rating", "", "rating filter is not supported by the v1 App API search contract")
	flags.StringVarP(&opts.typ, "type", "t", opts.typ, "entity type: artwork, novel, user")
	flags.StringVar(&opts.contentType, "content-type", opts.contentType, "artwork type filter: all, illust-and-ugoira, illust, manga, ugoira")
	flags.BoolVar(&opts.trendingTags, "trending-tags", false, "return the complete current artwork trending-tag list; WORD and list pagination are not accepted")
	flags.StringVar(&opts.resolution, "resolution", opts.resolution, "resolution filter: all, high, medium, low")
	flags.StringVar(&opts.aspectRatio, "aspect-ratio", opts.aspectRatio, "aspect ratio filter: all, landscape, portrait, square")
	flags.StringVar(&opts.drawTool, "draw-tool", "", "exact drawing tool name from the versioned drawing-tool catalog")
	flags.StringVar(&opts.aiMode, "ai-mode", opts.aiMode, "AI artwork filter: all, exclude, only")
	flags.IntVar(&opts.bookmarkMin, "bookmark-min", 0, "minimum public bookmark count; strategy controls filtering completeness")
	flags.IntVar(&opts.bookmarkMax, "bookmark-max", 0, "maximum public bookmark count; strategy controls filtering completeness")
	flags.StringVar(&opts.bookmarkStrategy, "bookmark-strategy", string(bookmarkFilterStrategyAuto), "bookmark count strategy: auto, local, best_effort, server")
	listing.BindListFlags(cmd, &opts.limit, &opts.page)
	data.bindTextValueWhen(cmd, 1, 1, 0, func(_ *cobra.Command, _ []string) bool { return !opts.trendingTags })
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

func (a command) run(cmd *cobra.Command, args []string, opts options) error {
	if opts.trendingTags {
		if err := validateTrendingTagsFlags(cmd); err != nil {
			return err
		}
		return a.runTrendingTags(cmd, opts.CommandOptions)
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
			CommandOptions: opts.CommandOptions,
			ndjson:         opts.ndjson,
			limit:          opts.limit,
			page:           opts.page,
			searchBy:       opts.searchBy,
			sortMode:       opts.sortMode,
			period:         opts.period,
		})
	}
	if entity == "user" {
		return user.RunSearch(cmd, a.userDependencies(), args, user.SearchOptions{
			CommandOptions: user.CommandOptions{Proxy: opts.Proxy, NoProxy: opts.NoProxy, JSON: opts.JSON},
			NDJSON:         opts.ndjson,
			Limit:          opts.limit,
			Page:           opts.page,
		})
	}
	if err := resolveRating(opts.rating); err != nil {
		return err
	}
	target, err := resolveSearchBy(opts.searchBy)
	if err != nil {
		return err
	}
	contentType, err := resolveContentType(opts.contentType)
	if err != nil {
		return err
	}
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
	plan, err := listing.ParsePlan(cmd, opts.limit, opts.page)
	if err != nil {
		return err
	}
	clientReq, err := a.data.request(cmd, opts.CommandOptions)
	if err != nil {
		return err
	}
	jsonOverride := a.data.jsonOverride(cmd, opts.CommandOptions)
	if opts.ndjson && cmd.Flags().Changed("json") {
		return a.data.usage(errors.New("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !opts.ndjson {
		jsonOut, err = a.data.jsonOut(jsonOverride)
		if err != nil {
			return err
		}
	}
	ndjson := a.data.shouldAutoNDJSON(cmd, opts.ndjson, jsonOut)
	query := pixiv.SearchArtworksRequest{
		Word: word, Target: target, Sort: pixiv.SortMode(opts.sortMode),
		Duration: period, StartDate: startDate, EndDate: endDate,
		ContentType: contentType, AIMode: aiMode, AspectRatio: aspectRatio, Resolution: resolution,
		Tool: opts.drawTool, BookmarkMin: bookmarkMin, BookmarkMax: bookmarkMax,
	}
	if bookmarkMin != nil || bookmarkMax != nil || cmd.Flags().Changed("bookmark-strategy") {
		if bookmarkMin == nil && bookmarkMax == nil {
			return errors.New("--bookmark-strategy requires --bookmark-min or --bookmark-max")
		}
		strategy, err := resolveBookmarkFilterStrategy(opts.bookmarkStrategy)
		if err != nil {
			return err
		}
		return a.runBookmarkFiltered(cmd, listing.Request(clientReq), plan, jsonOut, ndjson, word, query, strategy)
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
	return a.runner().RunPooledIllustList(cmd.Context(), listing.Request(clientReq), plan, jsonOut, ndjson, fmt.Sprintf("illustrations for %q", word), fetch,
		func(items []pixiv.Artwork, start int) error { return printArtworks(a.data.Output, items) })
}

func (a command) userDependencies() user.Dependencies {
	return user.Dependencies{
		Input:      a.data.Input,
		Output:     a.data.Output,
		UsageError: a.data.UsageError,
		JSONOut:    a.data.JSONOut,
		Pooled: func(ctx context.Context, request user.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			return a.data.Pooled(ctx, Request(request), attempt)
		},
	}
}

type artworkSearchOut struct {
	Illusts []pixiv.ArtworkDTO  `json:"illusts"`
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
func (a command) runBookmarkFiltered(cmd *cobra.Command, account listing.Request, plan listing.Plan, jsonOut, ndjson bool, word string, query pixiv.SearchArtworksRequest, strategy bookmarkFilterStrategy) error {
	outcome, err := searchArtworks(cmd.Context(), a.runner().Executor(account), artworkSearchRequest{
		Query:      query,
		Plan:       plan.PagePlan(),
		Membership: bookmarkMembershipUnknown,
		Strategy:   strategy,
	})
	if err != nil {
		return err
	}
	if ndjson {
		encoder := json.NewEncoder(a.data.Output)
		for _, item := range outcome.Page.Items {
			record, err := record.RecordFromArtworkDTO(pixiv.ToArtworkDTO(item))
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
		if outcome.Filter != nil {
			filter = &bookmarkFilterJSON{
				Min: outcome.Filter.Min, Max: outcome.Filter.Max,
				Membership: outcome.Filter.Membership, Strategy: outcome.Filter.Strategy,
				Completeness: outcome.Filter.Completeness,
			}
		}
		return a.data.writeJSON(artworkSearchOut{Illusts: artworkDTOs(outcome.Page.Items), Filter: filter})
	}
	if _, err := fmt.Fprintf(a.data.Output, "illustrations for %q\n", word); err != nil {
		return err
	}
	return printArtworks(a.data.Output, outcome.Page.Items)
}

func (a command) runTrendingTags(cmd *cobra.Command, options CommandOptions) error {
	request, err := a.data.request(cmd, options)
	if err != nil {
		return err
	}
	jsonOverride := a.data.jsonOverride(cmd, options)
	jsonOut, err := a.data.jsonOut(jsonOverride)
	if err != nil {
		return err
	}
	tags, err := read(a.data, cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) ([]pixiv.TrendingTag, error) {
		return client.TrendingArtworkTags(ctx, pixiv.TrendingArtworkTagsRequest{})
	})
	if err != nil {
		return err
	}
	if jsonOut {
		dtos := make([]pixiv.TrendingTagDTO, 0, len(tags))
		for _, tag := range tags {
			dtos = append(dtos, pixiv.ToTrendingTagDTO(tag))
		}
		return a.data.writeJSON(struct {
			Tags []pixiv.TrendingTagDTO `json:"tags"`
		}{Tags: dtos})
	}
	for _, tag := range tags {
		translated := tag.TranslatedName
		if translated == "" {
			translated = "none"
		}
		if _, err := fmt.Fprintf(a.data.Output, "%s (translation: %s)\n", tag.Tag, translated); err != nil {
			return err
		}
	}
	return nil
}

// runner is resolved only after Cobra has accepted the command input and the
// root has prepared the command's declared resources.
func (a command) runner() listing.Runner {
	return listing.New(a.data.Output, func(ctx context.Context, request listing.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
		return a.data.Pooled(ctx, Request(request), attempt)
	})
}

func artworkDTOs(items []pixiv.Artwork) []pixiv.ArtworkDTO {
	out := make([]pixiv.ArtworkDTO, 0, len(items))
	for _, item := range items {
		out = append(out, pixiv.ToArtworkDTO(item))
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

// resolveRating 保留合法值的诊断，但不把没有可靠接口证据的 rating 参数静默丢给
// public SDK。
func resolveRating(value string) error {
	if value == "" {
		return nil
	}
	switch value {
	case "sfw", "r18", "r18g", "mature", "all":
		return errors.New("rating filtering is not supported by the v1 App API search contract")
	default:
		return errors.New("rating must be one of sfw, r18, r18g, mature, all")
	}
}

func resolveContentType(value string) (pixiv.SearchContentType, error) {
	switch value {
	case "all", "illust-and-ugoira", "illust", "manga", "ugoira":
		return pixiv.SearchContentType(value), nil
	default:
		return "", errors.New("content-type must be one of all, illust-and-ugoira, illust, manga, ugoira")
	}
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
