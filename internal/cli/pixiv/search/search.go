// Package search owns the Pixiv artwork, novel and user search command routes.
package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	deps "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivdeps"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pipeline"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pixiv/internal/listing"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pixiv/user"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/requirements"
	searchpixiv "github.com/FlanChanXwO/pixiv-cli/internal/search/pixiv"
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
	deps.CommandOptions
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

type command struct {
	data deps.Data
}

// New builds the actual root `pixiv search` command.
func New(data deps.Data) *cobra.Command {
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
				return data.ExactArgs(0, "pixiv search --trending-tags [options]")(cmd, args)
			}
			return data.MinArgs(1, "pixiv search [options] WORD")(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.run(cmd, args, opts)
		},
	}
	data.BindCommonFlags(cmd, &opts.CommandOptions)
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
	flags.StringVar(&opts.bookmarkStrategy, "bookmark-strategy", string(searchpixiv.BookmarkFilterStrategyAuto), "bookmark count strategy: auto, local, best_effort, server")
	listing.BindListFlags(cmd, &opts.limit, &opts.page)
	data.BindTextValueWhen(cmd, 1, 1, 0, func(_ *cobra.Command, _ []string) bool { return !opts.trendingTags })
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
		return user.RunSearch(cmd, a.data, args, user.SearchOptions{
			CommandOptions: opts.CommandOptions,
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
	clientReq, err := a.data.Request(cmd, opts.CommandOptions)
	if err != nil {
		return err
	}
	jsonOverride := a.data.JSONOverride(cmd, opts.CommandOptions)
	if opts.ndjson && cmd.Flags().Changed("json") {
		return a.data.Usage(errors.New("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !opts.ndjson {
		jsonOut, err = a.data.JSONOut(jsonOverride)
		if err != nil {
			return err
		}
	}
	ndjson := a.data.ShouldAutoNDJSON(cmd, opts.ndjson, jsonOut)
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
		return a.runBookmarkFiltered(cmd, clientReq, plan, jsonOut, ndjson, word, query, strategy)
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
	return a.runner().RunPooledIllustList(cmd.Context(), clientReq, plan, jsonOut, ndjson, fmt.Sprintf("illustrations for %q", word), fetch,
		func(items []pixiv.Artwork, start int) error { return printArtworks(a.data.Output, items) })
}

type artworkSearchOut struct {
	Illusts []pixiv.ArtworkDTO  `json:"illusts"`
	Filter  *bookmarkFilterJSON `json:"filter,omitempty"`
}

type bookmarkFilterJSON struct {
	Min          *int                                   `json:"min,omitempty"`
	Max          *int                                   `json:"max,omitempty"`
	Membership   searchpixiv.BookmarkMembership         `json:"membership"`
	Strategy     searchpixiv.BookmarkFilterStrategy     `json:"strategy"`
	Completeness searchpixiv.BookmarkFilterCompleteness `json:"completeness"`
}

// runBookmarkFiltered 把 bookmark 区间过滤交给 search workflow；CLI 只保留输出
// 形态与过滤元数据的展示。
func (a command) runBookmarkFiltered(cmd *cobra.Command, account deps.Request, plan listing.Plan, jsonOut, ndjson bool, word string, query pixiv.SearchArtworksRequest, strategy searchpixiv.BookmarkFilterStrategy) error {
	outcome, err := searchpixiv.SearchArtworks(cmd.Context(), a.runner().PooledOperation(account), searchpixiv.ArtworkSearchRequest{
		Query:      query,
		Plan:       plan.PagePlan(),
		Membership: searchpixiv.BookmarkMembershipUnknown,
		Strategy:   strategy,
	})
	if err != nil {
		return err
	}
	if ndjson {
		encoder := json.NewEncoder(a.data.Output)
		for _, item := range outcome.Page.Items {
			record, err := pipeline.RecordFromArtworkDTO(pixiv.ToArtworkDTO(item))
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
		return a.data.WriteJSON(artworkSearchOut{Illusts: artworkDTOs(outcome.Page.Items), Filter: filter})
	}
	if _, err := fmt.Fprintf(a.data.Output, "illustrations for %q\n", word); err != nil {
		return err
	}
	return printArtworks(a.data.Output, outcome.Page.Items)
}

func (a command) runTrendingTags(cmd *cobra.Command, options deps.CommandOptions) error {
	request, err := a.data.Request(cmd, options)
	if err != nil {
		return err
	}
	jsonOverride := a.data.JSONOverride(cmd, options)
	jsonOut, err := a.data.JSONOut(jsonOverride)
	if err != nil {
		return err
	}
	tags, err := deps.Read(a.data, cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) ([]pixiv.TrendingTag, error) {
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
		return a.data.WriteJSON(struct {
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
	return listing.New(a.data.Output, a.data)
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

func resolveBookmarkFilterStrategy(value string) (searchpixiv.BookmarkFilterStrategy, error) {
	strategy := searchpixiv.BookmarkFilterStrategy(value)
	switch strategy {
	case searchpixiv.BookmarkFilterStrategyAuto, searchpixiv.BookmarkFilterStrategyLocal, searchpixiv.BookmarkFilterStrategyBestEffort, searchpixiv.BookmarkFilterStrategyServer:
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
	if startDate, endDate, ok := searchpixiv.QuickDateRange(string(period), time.Now()); ok {
		return "", startDate, endDate, nil
	}
	return period, startDate, endDate, nil
}

func validDate(value string) bool {
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}
