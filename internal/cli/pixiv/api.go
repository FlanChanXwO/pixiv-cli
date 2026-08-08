package pixiv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	pixivapp "github.com/FlanChanXwO/pixiv-cli/internal/application/pixiv"
	clioutput "github.com/FlanChanXwO/pixiv-cli/internal/cli/output"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

type searchOptions struct {
	commandOptions
	ndjsonOutputOptions
	searchBy    string
	sortMode    string
	period      string
	startDate   string
	endDate     string
	resolution  string
	aspectRatio string
	drawTool    string
	aiMode      string
	listOptions
	rating      string
	typ         string
	bookmarkMin int
	bookmarkMax int
}

type rankingOptions struct {
	commandOptions
	ndjsonOutputOptions
	mode string
	date string
	listOptions
}

type recommendedOptions struct {
	commandOptions
	ndjsonOutputOptions
	listOptions
}

const (
	searchTargetTagPartial      = "tag-partial"
	searchTargetTagExact        = "tag-exact"
	searchTargetTitleCaption    = "title-caption"
	searchTargetTagTitleCaption = "tag-title-caption"
)

func (a controller) newSearchCommand() *cobra.Command {
	opts := searchOptions{
		searchBy:    searchTargetTagPartial,
		sortMode:    string(pixiv.SortModeDateDesc),
		rating:      "all",
		typ:         "all",
		resolution:  string(pixiv.SearchResolutionAll),
		aspectRatio: string(pixiv.SearchAspectRatioAll),
		aiMode:      string(pixiv.SearchAIModeAll),
	}
	cmd := &cobra.Command{
		Use:     "search WORD",
		Short:   "Search illustrations",
		Example: "pixiv search \"miku\" --json",
		Args:    a.requireMinArgs(1, "pixiv search [options] WORD"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runSearch(cmd, args, opts)
		},
	}
	a.bindCommonFlags(cmd, &opts.commandOptions)
	bindNDJSONFlag(cmd, &opts.ndjsonOutputOptions)
	flags := cmd.Flags()
	flags.StringVar(&opts.searchBy, "search-by", opts.searchBy, "search field: tag-partial, tag-exact, title-caption, tag-title-caption")
	flags.StringVar(&opts.sortMode, "sort", opts.sortMode, "sort mode: date_desc, date_asc")
	flags.StringVar(&opts.period, "period", "", "time range: day, week, month, half-year, year")
	flags.StringVar(&opts.startDate, "start-date", "", "inclusive start date: YYYY-MM-DD")
	flags.StringVar(&opts.endDate, "end-date", "", "inclusive end date: YYYY-MM-DD")
	flags.StringVar(&opts.rating, "rating", opts.rating, "rating filter: sfw, r18, r18g, mature, all (accepted for compatibility; upstream App API search no longer filters by rating)")
	flags.StringVar(&opts.typ, "type", opts.typ, "artwork type filter: all, illust-and-ugoira, illust, manga, ugoira")
	flags.StringVar(&opts.resolution, "resolution", opts.resolution, "resolution filter: all, high, medium, low")
	flags.StringVar(&opts.aspectRatio, "aspect-ratio", opts.aspectRatio, "aspect ratio filter: all, landscape, portrait, square")
	flags.StringVar(&opts.drawTool, "draw-tool", "", "exact drawing tool name from the versioned drawing-tool catalog")
	flags.StringVar(&opts.aiMode, "ai-mode", opts.aiMode, "AI artwork filter: all, exclude, only")
	flags.IntVar(&opts.bookmarkMin, "bookmark-min", 0, "minimum public bookmark count (requires Pixiv Premium)")
	flags.IntVar(&opts.bookmarkMax, "bookmark-max", 0, "maximum public bookmark count (requires Pixiv Premium)")
	bindListFlags(cmd, &opts.listOptions)
	return cmd
}

func (a controller) runSearch(cmd *cobra.Command, args []string, opts searchOptions) error {
	if err := resolveSearchRating(opts.rating); err != nil {
		return err
	}
	target, err := resolveSearchBy(opts.searchBy)
	if err != nil {
		return err
	}
	contentType, err := resolveSearchContentType(opts.typ)
	if err != nil {
		return err
	}
	aiMode, err := resolveSearchAIMode(opts.aiMode)
	if err != nil {
		return err
	}
	aspectRatio, err := resolveSearchAspectRatio(opts.aspectRatio)
	if err != nil {
		return err
	}
	resolution, err := resolveSearchResolution(opts.resolution)
	if err != nil {
		return err
	}
	period, err := resolveSearchPeriod(opts.period)
	if err != nil {
		return err
	}
	period, startDate, endDate, err := resolveSearchDateRange(opts, period)
	if err != nil {
		return err
	}
	bookmarkMin, bookmarkMax, err := resolveSearchBookmarkRange(cmd, opts)
	if err != nil {
		return err
	}
	word := strings.Join(args, " ")
	plan, err := parseListPlan(cmd, opts.listOptions)
	if err != nil {
		return err
	}
	services := a.services()
	clientReq, jsonOverride, err := a.sdkRequest(cmd, opts.commandOptions)
	if err != nil {
		return err
	}
	if opts.ndjson && cmd.Flags().Changed("json") {
		return a.usageError(errors.New("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !opts.ndjson {
		jsonOut, err = services.SDK.JSONOut(jsonOverride)
		if err != nil {
			return err
		}
	}
	opts.ndjson = a.shouldAutoNDJSON(cmd, opts.ndjson, jsonOut)
	fetch := func(client pixivapp.ClientSet, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		result, err := client.SearchArtworks(ctx, pixiv.SearchArtworksRequest{
			Word: word, Target: target, Sort: pixiv.SortMode(opts.sortMode),
			Duration: period, StartDate: startDate, EndDate: endDate,
			ContentType: contentType, AIMode: aiMode, AspectRatio: aspectRatio, Resolution: resolution,
			Tool: opts.drawTool, BookmarkMin: bookmarkMin, BookmarkMax: bookmarkMax, Cursor: cursor,
		})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return a.runPooledIllustList(cmd.Context(), clientReq, plan, jsonOut, opts.ndjson, fmt.Sprintf("illustrations for %q", word), fetch, func(items []pixiv.Artwork, start int) error { return printIllusts(a.out, items, start, false) })
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

// resolveSearchRating 只校验兼容保留的 --rating 取值；新版 App API 搜索不再支持
// rating 过滤，因此校验通过后不映射到请求。
func resolveSearchRating(value string) error {
	switch value {
	case "sfw", "r18", "r18g", "mature", "all":
		return nil
	default:
		return fmt.Errorf("rating must be one of sfw, r18, r18g, mature, all")
	}
}

func resolveSearchContentType(value string) (pixiv.SearchContentType, error) {
	switch value {
	case "all", "illust-and-ugoira", "illust", "manga", "ugoira":
		return pixiv.SearchContentType(value), nil
	default:
		return "", fmt.Errorf("type must be one of all, illust-and-ugoira, illust, manga, ugoira")
	}
}

func resolveSearchAIMode(value string) (pixiv.SearchAIMode, error) {
	switch value {
	case "all", "exclude", "only":
		return pixiv.SearchAIMode(value), nil
	default:
		return "", fmt.Errorf("ai-mode must be one of all, exclude, only")
	}
}

func resolveSearchAspectRatio(value string) (pixiv.SearchAspectRatio, error) {
	switch value {
	case "all", "landscape", "portrait", "square":
		return pixiv.SearchAspectRatio(value), nil
	default:
		return "", fmt.Errorf("aspect-ratio must be one of all, landscape, portrait, square")
	}
}

func resolveSearchResolution(value string) (pixiv.SearchResolution, error) {
	switch value {
	case "all", "high", "medium", "low":
		return pixiv.SearchResolution(value), nil
	default:
		return "", fmt.Errorf("resolution must be one of all, high, medium, low")
	}
}

func resolveSearchBookmarkRange(cmd *cobra.Command, opts searchOptions) (*int, *int, error) {
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

func resolveSearchPeriod(value string) (pixiv.DurationFilter, error) {
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

func resolveSearchDateRange(opts searchOptions, period pixiv.DurationFilter) (pixiv.DurationFilter, string, string, error) {
	startDate := strings.TrimSpace(opts.startDate)
	endDate := strings.TrimSpace(opts.endDate)
	if period != "" && (startDate != "" || endDate != "") {
		return "", "", "", errors.New("period cannot be combined with start-date or end-date")
	}
	if startDate != "" && !validSearchDate(startDate) || endDate != "" && !validSearchDate(endDate) {
		return "", "", "", errors.New("start-date and end-date must use YYYY-MM-DD")
	}
	if startDate != "" && endDate != "" && startDate > endDate {
		return "", "", "", errors.New("start-date cannot be later than end-date")
	}
	if startDate, endDate, ok := pixivapp.SearchQuickDateRange(string(period), time.Now()); ok {
		return "", startDate, endDate, nil
	}
	return period, startDate, endDate, nil
}

func validSearchDate(value string) bool {
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}

// parseArtworkIDOrURL 解析 detail 命令的位置参数：裸正整型视为作品 ID，
// 其余必须是可解析到作品的 Pixiv URL。
func parseArtworkIDOrURL(arg string) (int64, error) {
	value := strings.TrimSpace(arg)
	if id, err := strconv.ParseInt(value, 10, 64); err == nil && id > 0 {
		return id, nil
	}
	ref, err := pixiv.ParseURL(value)
	if err != nil {
		return 0, errors.New("argument must be an illustration ID or a supported Pixiv URL")
	}
	if ref.Kind != pixiv.ReferenceKindArtwork {
		return 0, errors.New("URL does not name a supported Pixiv artwork")
	}
	return ref.ID, nil
}

func (a controller) newDetailCommand() *cobra.Command {
	var opts commandOptions
	cmd := &cobra.Command{
		Use:   "detail ILLUST_ID_OR_URL",
		Short: "Show one illustration",
		Args:  a.requireExactArgs(1, "pixiv detail [options] ILLUST_ID_OR_URL"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runDetail(cmd, args[0], opts)
		},
	}
	a.bindCommonFlags(cmd, &opts)
	return cmd
}

func (a controller) runDetail(cmd *cobra.Command, arg string, opts commandOptions) error {
	id, err := parseArtworkIDOrURL(arg)
	if err != nil {
		return err
	}
	services := a.services()
	clientReq, jsonOverride, err := a.sdkRequest(cmd, opts)
	if err != nil {
		return err
	}
	jsonOut, err := services.SDK.JSONOut(jsonOverride)
	if err != nil {
		return err
	}
	return services.SDK.RunPooledOperation(cmd.Context(), clientReq, func(ctx context.Context, client pixivapp.ClientSet) (bool, error) {
		result, err := client.Artwork(ctx, pixiv.ArtworkRequest{ArtworkID: id})
		if err != nil {
			return false, err
		}
		if jsonOut {
			committed := true
			err = a.printJSON(result)
			return committed, err
		} else {
			committed := true
			err = printIllust(a.out, result, 0, false)
			return committed, err
		}
	})
}

func (a controller) newRankingCommand() *cobra.Command {
	opts := rankingOptions{mode: string(pixiv.RankingModeDay)}
	cmd := &cobra.Command{
		Use:   "ranking",
		Short: "Show illustration ranking",
		Args:  a.requireExactArgs(0, "pixiv ranking [options]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runRanking(cmd, opts)
		},
	}
	a.bindCommonFlags(cmd, &opts.commandOptions)
	bindNDJSONFlag(cmd, &opts.ndjsonOutputOptions)
	flags := cmd.Flags()
	flags.StringVar(&opts.mode, "mode", opts.mode, "ranking mode: day, day_male, day_female, week, week_original, week_rookie, month, day_manga, week_manga, month_manga, week_rookie_manga, day_r18, day_male_r18, day_female_r18, week_r18, week_r18g; the last nine require authentication")
	flags.StringVar(&opts.date, "date", "", "YYYY-MM-DD")
	bindListFlags(cmd, &opts.listOptions)
	return cmd
}

func (a controller) runRanking(cmd *cobra.Command, opts rankingOptions) error {
	plan, err := parseListPlan(cmd, opts.listOptions)
	if err != nil {
		return err
	}
	services := a.services()
	clientReq, jsonOverride, err := a.sdkRequest(cmd, opts.commandOptions)
	if err != nil {
		return err
	}
	if opts.ndjson && cmd.Flags().Changed("json") {
		return a.usageError(errors.New("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !opts.ndjson {
		jsonOut, err = services.SDK.JSONOut(jsonOverride)
		if err != nil {
			return err
		}
	}
	opts.ndjson = a.shouldAutoNDJSON(cmd, opts.ndjson, jsonOut)
	fetch := func(client pixivapp.ClientSet, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		result, err := client.ArtworkRanking(ctx, pixiv.ArtworkRankingRequest{Mode: pixiv.RankingMode(opts.mode), Date: opts.date, Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return a.runPooledIllustList(cmd.Context(), clientReq, plan, jsonOut, opts.ndjson, fmt.Sprintf("%s ranking", opts.mode), fetch, func(items []pixiv.Artwork, start int) error { return printIllusts(a.out, items, start, true) })
}

func (a controller) newRecommendedCommand() *cobra.Command {
	opts := recommendedOptions{}
	cmd := &cobra.Command{
		Use:   "recommended all|illust|manga|novel|user",
		Short: "Show personalized recommendations",
		Args:  a.requireExactArgs(1, "pixiv recommended all|illust|manga|novel|user [options]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runRecommended(cmd, args[0], opts)
		},
	}
	a.bindCommonFlags(cmd, &opts.commandOptions)
	bindNDJSONFlag(cmd, &opts.ndjsonOutputOptions)
	bindListFlags(cmd, &opts.listOptions)
	return cmd
}

func printIllusts(w io.Writer, illusts []pixiv.Artwork, offset int, ranked bool) error {
	for i, illust := range illusts {
		rank := 0
		if ranked {
			rank = i + 1 + offset
		}
		if err := printIllust(w, illust, rank, true); err != nil {
			return err
		}
	}
	return nil
}

func printIllust(w io.Writer, illust pixiv.Artwork, rank int, compact bool) error {
	prefix := ""
	if rank > 0 {
		prefix = fmt.Sprintf("#%d ", rank)
	}
	tags := make([]string, 0, len(illust.Tags))
	for _, tag := range illust.Tags {
		tags = append(tags, tag.Name)
	}
	url := ""
	if illust.ID > 0 {
		url = fmt.Sprintf("https://www.pixiv.net/artworks/%d", illust.ID)
	}
	if compact {
		if _, err := fmt.Fprintf(w, "%s%s\n", prefix, url); err != nil {
			return err
		}
		_, err := fmt.Fprintf(w, "%d %q by %s bookmarks:%d views:%d tags:%s\n",
			illust.ID, illust.Title, illust.User.Name, illust.TotalBookmarks, illust.TotalViews, strings.Join(tags, ","))
		return err
	}
	for _, line := range []string{
		fmt.Sprintf("url: %s\n", url), fmt.Sprintf("id: %d\n", illust.ID), fmt.Sprintf("title: %s\n", illust.Title),
		fmt.Sprintf("author: %s (%d)\n", illust.User.Name, illust.User.ID), fmt.Sprintf("type: %s\n", string(illust.Kind)),
		fmt.Sprintf("page_count: %d\n", illust.PageCount), fmt.Sprintf("bookmarks: %d\n", illust.TotalBookmarks),
		fmt.Sprintf("views: %d\n", illust.TotalViews), fmt.Sprintf("tags: %s\n", strings.Join(tags, ",")),
	} {
		if _, err := io.WriteString(w, line); err != nil {
			return err
		}
	}
	if caption := clioutput.CaptionPlainText(illust.Caption); caption != "" {
		if _, err := fmt.Fprintf(w, "caption:\n%s\n", caption); err != nil {
			return err
		}
	}
	return nil
}
