package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
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

func (a app) newSearchCommand() *cobra.Command {
	opts := searchOptions{
		searchBy:    searchTargetTagPartial,
		sortMode:    string(sdk.SortModeDateDesc),
		rating:      "all",
		typ:         "all",
		resolution:  string(sdk.SearchResolutionAll),
		aspectRatio: string(sdk.SearchAspectRatioAll),
		aiMode:      string(sdk.SearchAIModeAll),
	}
	cmd := &cobra.Command{
		Use:     "search WORD",
		Short:   "Search illustrations",
		Example: "pixiv search \"miku\" --json",
		Args:    requireMinArgs(1, "pixiv search [options] WORD"),
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
	flags.StringVar(&opts.rating, "rating", opts.rating, "rating filter: sfw, r18, r18g, mature, all")
	flags.StringVar(&opts.typ, "type", opts.typ, "artwork type filter: all, illust-and-ugoira, illust, manga, ugoira")
	flags.StringVar(&opts.resolution, "resolution", opts.resolution, "resolution filter: all, high, medium, low")
	flags.StringVar(&opts.aspectRatio, "aspect-ratio", opts.aspectRatio, "aspect ratio filter: all, landscape, portrait, square")
	flags.StringVar(&opts.drawTool, "draw-tool", "", "drawing tool name from search-options")
	flags.StringVar(&opts.aiMode, "ai-mode", opts.aiMode, "AI artwork filter: all, exclude, only")
	flags.IntVar(&opts.bookmarkMin, "bookmark-min", 0, "minimum public bookmark count (requires Pixiv Premium)")
	flags.IntVar(&opts.bookmarkMax, "bookmark-max", 0, "maximum public bookmark count (requires Pixiv Premium)")
	bindListFlags(cmd, &opts.listOptions)
	return cmd
}

func (a app) runSearch(cmd *cobra.Command, args []string, opts searchOptions) error {
	filters, err := resolveSearchFilters(cmd, opts)
	if err != nil {
		return err
	}
	target, err := resolveSearchBy(opts.searchBy)
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
		return newUsageError(errors.New("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !opts.ndjson {
		jsonOut, err = services.SDK.JSONOut(jsonOverride)
		if err != nil {
			return err
		}
	}
	fetch := func(client application.SDKClient, ctx context.Context, cursor sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
		result, err := client.SearchIllust(ctx, sdk.SearchIllustRequest{
			Word: word, Target: target, Sort: sdk.SortMode(opts.sortMode),
			Duration: period, StartDate: startDate, EndDate: endDate, Cursor: cursor, Filters: filters,
		})
		if err != nil {
			return nil, "", err
		}
		return result.Illusts, result.NextCursor, nil
	}
	return a.runPooledIllustList(cmd.Context(), clientReq, plan, jsonOut, opts.ndjson, fmt.Sprintf("illustrations for %q", word), fetch, func(items []sdk.Illust, start int) error { return printIllusts(a.out, items, start, false) })
}

func resolveSearchBy(value string) (sdk.SearchTarget, error) {
	switch value {
	case searchTargetTagPartial:
		return sdk.SearchTargetPartialMatchForTags, nil
	case searchTargetTagExact:
		return sdk.SearchTargetExactMatchForTags, nil
	case searchTargetTitleCaption:
		return sdk.SearchTargetTitleAndCaption, nil
	case searchTargetTagTitleCaption:
		return sdk.SearchTargetKeyword, nil
	default:
		return "", fmt.Errorf("search-by must be one of %s, %s, %s, %s", searchTargetTagPartial, searchTargetTagExact, searchTargetTitleCaption, searchTargetTagTitleCaption)
	}
}

func resolveSearchPeriod(value string) (string, error) {
	switch value {
	case "":
		return "", nil
	case "day":
		return "within_last_day", nil
	case "week":
		return "within_last_week", nil
	case "month":
		return "within_last_month", nil
	case "half-year":
		return "within_half_year", nil
	case "year":
		return "within_year", nil
	default:
		return "", errors.New("period must be one of day, week, month, half-year, year")
	}
}

func resolveSearchDateRange(opts searchOptions, period string) (string, string, string, error) {
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
	if startDate, endDate, ok := application.SearchQuickDateRange(period, time.Now()); ok {
		return "", startDate, endDate, nil
	}
	return period, startDate, endDate, nil
}

func validSearchDate(value string) bool {
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}

func resolveSearchFilters(cmd *cobra.Command, opts searchOptions) (sdk.SearchIllustFilters, error) {
	filters := sdk.SearchIllustFilters{}
	switch opts.rating {
	case "sfw", "r18", "r18g", "mature", "all":
		filters.Rating = sdk.SearchRating(opts.rating)
	default:
		return filters, fmt.Errorf("rating must be one of sfw, r18, r18g, mature, all")
	}
	switch opts.typ {
	case "all", "illust-and-ugoira", "illust", "manga", "ugoira":
		filters.ContentType = sdk.SearchContentType(opts.typ)
	default:
		return filters, fmt.Errorf("type must be one of all, illust-and-ugoira, illust, manga, ugoira")
	}
	switch opts.resolution {
	case "all", "high", "medium", "low":
		filters.Resolution = sdk.SearchResolution(opts.resolution)
	default:
		return filters, fmt.Errorf("resolution must be one of all, high, medium, low")
	}
	switch opts.aspectRatio {
	case "all", "landscape", "portrait", "square":
		filters.AspectRatio = sdk.SearchAspectRatio(opts.aspectRatio)
	default:
		return filters, fmt.Errorf("aspect-ratio must be one of all, landscape, portrait, square")
	}
	switch opts.aiMode {
	case "all", "exclude", "only":
		filters.AIMode = sdk.SearchAIMode(opts.aiMode)
	default:
		return filters, fmt.Errorf("ai-mode must be one of all, exclude, only")
	}
	filters.Tool = opts.drawTool
	if cmd.Flags().Changed("bookmark-min") {
		if opts.bookmarkMin < 0 {
			return filters, errors.New("bookmark-min must be greater than or equal to zero")
		}
		minimum := opts.bookmarkMin
		filters.BookmarkMin = &minimum
	}
	if cmd.Flags().Changed("bookmark-max") {
		if opts.bookmarkMax < 0 {
			return filters, errors.New("bookmark-max must be greater than or equal to zero")
		}
		maximum := opts.bookmarkMax
		filters.BookmarkMax = &maximum
	}
	if filters.BookmarkMin != nil && filters.BookmarkMax != nil && *filters.BookmarkMin > *filters.BookmarkMax {
		return filters, errors.New("bookmark-min cannot be greater than bookmark-max")
	}
	return filters, nil
}

func (a app) newSearchOptionsCommand() *cobra.Command {
	var opts commandOptions
	cmd := &cobra.Command{
		Use:     "search-options WORD",
		Short:   "Show available illustration search options",
		Example: "pixiv search-options \"miku\" --json",
		Args:    requireMinArgs(1, "pixiv search-options [options] WORD"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runSearchOptions(cmd, strings.Join(args, " "), opts)
		},
	}
	a.bindCommonFlags(cmd, &opts)
	return cmd
}

func (a app) runSearchOptions(cmd *cobra.Command, word string, opts commandOptions) error {
	services := a.services()
	clientReq, jsonOverride, err := a.sdkRequest(cmd, opts)
	if err != nil {
		return err
	}
	jsonOut, err := services.SDK.JSONOut(jsonOverride)
	if err != nil {
		return err
	}
	return services.SDK.RunPooledOperation(cmd.Context(), clientReq, func(ctx context.Context, client application.SDKClient) (bool, error) {
		result, err := client.SearchIllustOptions(ctx, sdk.SearchIllustOptionsRequest{Word: word})
		if err != nil {
			return false, err
		}
		if jsonOut {
			committed := true
			err = a.printJSON(result)
			return committed, err
		}
		committed := true
		if _, err := fmt.Fprintf(a.out, "search options for %q\n", word); err != nil {
			return committed, err
		}
		if len(result.Tools) == 0 {
			_, err = fmt.Fprintln(a.out, "tools: none")
			return committed, err
		}
		if _, err := fmt.Fprintln(a.out, "tools:"); err != nil {
			return committed, err
		}
		for _, tool := range result.Tools {
			if _, err := fmt.Fprintf(a.out, "- %s\n", safeTextLine(tool)); err != nil {
				return committed, err
			}
		}
		return committed, nil
	})
}

// safeTextLine 保留可见 Unicode，同时转义换行、ANSI ESC 和其他控制字节，
// 防止上游工具名破坏终端的逐行文本协议。JSON 输出仍交由 encoding/json 原样编码。
func safeTextLine(value string) string {
	quoted := strconv.QuoteToGraphic(value)
	return quoted[1 : len(quoted)-1]
}

func (a app) newDetailCommand() *cobra.Command {
	var opts commandOptions
	cmd := &cobra.Command{
		Use:   "detail ILLUST_ID_OR_URL",
		Short: "Show one illustration",
		Args:  requireExactArgs(1, "pixiv detail [options] ILLUST_ID_OR_URL"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runDetail(cmd, args[0], opts)
		},
	}
	a.bindCommonFlags(cmd, &opts)
	return cmd
}

func (a app) runDetail(cmd *cobra.Command, arg string, opts commandOptions) error {
	id, err := sdk.ParseArtworkReference(arg)
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
	return services.SDK.RunPooledOperation(cmd.Context(), clientReq, func(ctx context.Context, client application.SDKClient) (bool, error) {
		result, err := client.IllustDetail(ctx, id)
		if err != nil {
			return false, err
		}
		if jsonOut {
			committed := true
			err = a.printJSON(result)
			return committed, err
		} else {
			committed := true
			err = printIllust(a.out, result.Illust, 0, false)
			return committed, err
		}
	})
}

func (a app) newRankingCommand() *cobra.Command {
	opts := rankingOptions{mode: string(sdk.RankingModeDay)}
	cmd := &cobra.Command{
		Use:   "ranking",
		Short: "Show illustration ranking",
		Args:  requireExactArgs(0, "pixiv ranking [options]"),
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

func (a app) runRanking(cmd *cobra.Command, opts rankingOptions) error {
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
		return newUsageError(errors.New("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !opts.ndjson {
		jsonOut, err = services.SDK.JSONOut(jsonOverride)
		if err != nil {
			return err
		}
	}
	fetch := func(client application.SDKClient, ctx context.Context, cursor sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
		result, err := client.IllustRanking(ctx, sdk.IllustRankingRequest{Mode: sdk.RankingMode(opts.mode), Date: opts.date, Cursor: cursor})
		if err != nil {
			return nil, "", err
		}
		return result.Illusts, result.NextCursor, nil
	}
	return a.runPooledIllustList(cmd.Context(), clientReq, plan, jsonOut, opts.ndjson, fmt.Sprintf("%s ranking", opts.mode), fetch, func(items []sdk.Illust, start int) error { return printIllusts(a.out, items, start, true) })
}

func (a app) newRecommendedCommand() *cobra.Command {
	opts := recommendedOptions{}
	cmd := &cobra.Command{
		Use:   "recommended all|illust|manga|novel|user",
		Short: "Show personalized recommendations",
		Args:  requireExactArgs(1, "pixiv recommended all|illust|manga|novel|user [options]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runRecommended(cmd, args[0], opts)
		},
	}
	a.bindCommonFlags(cmd, &opts.commandOptions)
	bindNDJSONFlag(cmd, &opts.ndjsonOutputOptions)
	bindListFlags(cmd, &opts.listOptions)
	return cmd
}

type downloadOptions struct {
	commandOptions
	downloadPath     string
	filenameTemplate string
	pages            string
	quality          string
	ugoiraFormat     string
	concurrency      int
	onError          string
}

func (a app) newDownloadCommand() *cobra.Command {
	opts := downloadOptions{quality: string(application.DownloadQualityOriginal)}
	cmd := &cobra.Command{
		Use:   "download [SRC...]",
		Short: "Download illustrations",
		Args:  actionOrTargetsArgs(a.in, "pixiv download [options] SRC..."),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runDownload(cmd, args, opts)
		},
	}
	a.bindActionFlags(cmd, &opts.commandOptions)
	a.bindDownloadRuntimeFlags(cmd, &opts)
	flags := cmd.Flags()
	flags.StringVar(&opts.pages, "pages", "", "1-based page selection, e.g. 1,3-5; default all pages")
	flags.StringVar(&opts.quality, "quality", opts.quality, "static image quality: original, regular, small, thumb, mini")
	flags.StringVar(&opts.ugoiraFormat, "ugoira-format", string(sdk.UgoiraFormatGIF), "ugoira output format: gif or apng")
	flags.IntVar(&opts.concurrency, "concurrency", 0, "download workers; 0 automatically uses 2 × GOMAXPROCS")
	flags.StringVar(&opts.onError, "on-error", "skip", "record failure strategy: skip or fail-fast")
	return cmd
}

// bindDownloadRuntimeFlags 只注册真正影响下载落盘的参数，避免非下载命令静默接受无效 flag。
func (a app) bindDownloadRuntimeFlags(cmd *cobra.Command, opts *downloadOptions) {
	flags := cmd.Flags()
	flags.StringVar(&opts.downloadPath, "download-path", "", "download directory")
	flags.StringVar(&opts.filenameTemplate, "filename-template", "", "filename template placeholders: {id}, {title}, {author}")
}

func (a app) runDownload(cmd *cobra.Command, args []string, opts downloadOptions) error {
	if _, err := recordFailureStrategy(opts.onError); err != nil {
		return err
	}
	pages, err := application.ParsePageSpec(opts.pages)
	if err != nil {
		return newUsageError(err)
	}
	quality := application.DownloadQuality(opts.quality)
	if quality == "" {
		quality = application.DownloadQualityOriginal
	}
	if err := application.ValidateDownloadQuality(quality); err != nil {
		return newUsageError(err)
	}
	ugoiraFormat := sdk.UgoiraFormat(opts.ugoiraFormat)
	if err := sdk.ValidateUgoiraFormat(ugoiraFormat); err != nil {
		return newUsageError(err)
	}
	services := a.services()
	clientReq, err := a.clientRequest(cmd, opts.commandOptions, false)
	if err != nil {
		return err
	}
	runtime, err := services.SDK.Runtime()
	if err != nil {
		return err
	}
	if cmd.Flags().Changed("download-path") {
		runtime.DownloadPath = opts.downloadPath
	}
	if cmd.Flags().Changed("filename-template") {
		runtime.FilenameTemplate = opts.filenameTemplate
	}
	options := sdk.DownloadOptions{
		DownloadPath:     runtime.DownloadPath,
		FilenameTemplate: runtime.FilenameTemplate,
		Pages:            pages,
		Quality:          sdk.DownloadQuality(quality),
		UgoiraFormat:     ugoiraFormat,
		Concurrency:      opts.concurrency,
	}
	request := application.SDKClientRequest{HTTPSProxyOverride: clientReq.HTTPSProxyOverride}
	downloadOne := func(ctx context.Context, id int64) error {
		return runPooledSingleDownload(ctx, services, request, id, options)
	}
	if len(args) == 0 {
		return a.consumeActionRecords(cmd, "download", opts.onError, visualRecordTypes, downloadOne)
	}
	allArtworkReferences := true
	for _, source := range args {
		reference, parseErr := sdk.ParseReference(source)
		if parseErr != nil || reference.Kind != sdk.ReferenceKindArtwork {
			allArtworkReferences = false
			break
		}
	}
	if allArtworkReferences {
		for _, source := range args {
			reference, _ := sdk.ParseReference(source)
			if err := downloadOne(cmd.Context(), reference.ID); err != nil {
				return err
			}
		}
		return nil
	}
	// 用户页与受资源策略允许的直链可能在一次调用里展开多个文件。只在结果中
	// 尚无已发布文件时，账号池才可因有效 Retry-After 安全重放这次调用。
	return services.SDK.RunPooledOperation(cmd.Context(), request, func(ctx context.Context, client application.SDKClient) (bool, error) {
		report, err := services.Download.DownloadSources(ctx, client, args, options)
		if err != nil {
			return downloadReportCommitted(report), err
		}
		return downloadReportCommitted(report), downloadReportError(report)
	})
}

// runPooledSingleDownload 将单个作品作为下载的提交单元。只要已经落盘任一文件，
// 后续错误均不允许账号池用另一账号重放该作品。
func runPooledSingleDownload(ctx context.Context, services application.Services, request application.SDKClientRequest, id int64, options sdk.DownloadOptions) error {
	return services.SDK.RunPooledOperation(ctx, request, func(ctx context.Context, client application.SDKClient) (bool, error) {
		report, err := services.Download.DownloadSources(ctx, client, []string{strconv.FormatInt(id, 10)}, options)
		if err != nil {
			return false, err
		}
		return downloadReportCommitted(report), downloadReportError(report)
	})
}

// downloadReportCommitted 只以已经原子发布的常规文件判断提交边界。资源获取
// 或解析在此之前失败时，账号池可识别有效 Retry-After 并切换未尝试账号。
func downloadReportCommitted(report application.DownloadReport) bool {
	if report.Committed {
		return true
	}
	for _, item := range report.Items {
		if len(item.Files) > 0 {
			return true
		}
	}
	return false
}

func downloadReportError(report application.DownloadReport) error {
	if len(report.Failures) == 0 {
		return nil
	}
	first := report.Failures[0]
	if first.Cause != nil {
		return fmt.Errorf("download completed with %d failures: %w", len(report.Failures), first.Cause)
	}
	if first.Message == "" {
		return fmt.Errorf("download completed with %d failures", len(report.Failures))
	}
	return fmt.Errorf("download completed with %d failures: %s", len(report.Failures), first.Message)
}

// downloadReportOut 是 CLI 的稳定 JSON 下载报告；作品 URL 始终按 ID 重建为规范地址。
type downloadReportOut struct {
	Items    []downloadArtworkOut `json:"items"`
	Failures []downloadFailureOut `json:"failures"`
}

type downloadArtworkOut struct {
	URL      string            `json:"url"`
	IllustID int64             `json:"illust_id"`
	Title    string            `json:"title"`
	Author   string            `json:"author"`
	Type     string            `json:"type"`
	Files    []downloadFileOut `json:"files"`
}

type downloadFileOut struct {
	Path string `json:"path"`
	Page int    `json:"page"`
}

type downloadFailureOut struct {
	URL      string `json:"url"`
	IllustID int64  `json:"illust_id"`
	Type     string `json:"type"`
	Message  string `json:"message"`
}

func downloadReportOutput(report application.DownloadReport) downloadReportOut {
	out := downloadReportOut{Items: []downloadArtworkOut{}, Failures: []downloadFailureOut{}}
	for _, artwork := range report.Items {
		item := downloadArtworkOut{
			URL:      sdk.Reference{Kind: sdk.ReferenceKindArtwork, ID: artwork.IllustID}.URL(),
			IllustID: artwork.IllustID,
			Title:    artwork.Title,
			Author:   artwork.Author,
			Type:     artwork.Type,
			Files:    []downloadFileOut{},
		}
		for _, file := range artwork.Files {
			item.Files = append(item.Files, downloadFileOut{Path: file.Path, Page: file.Page})
		}
		out.Items = append(out.Items, item)
	}
	for _, failure := range report.Failures {
		out.Failures = append(out.Failures, downloadFailureOut{
			URL: failure.URL, IllustID: failure.IllustID, Type: failure.Type, Message: failure.Message,
		})
	}
	return out
}

func printDownloadReport(w io.Writer, report application.DownloadReport) error {
	for _, artwork := range report.Items {
		if _, err := fmt.Fprintf(w, "downloaded %d %q by %s\n", artwork.IllustID, artwork.Title, artwork.Author); err != nil {
			return err
		}
		for _, file := range artwork.Files {
			if _, err := fmt.Fprintf(w, "  %s\n", file.Path); err != nil {
				return err
			}
		}
	}
	for _, failure := range report.Failures {
		if _, err := fmt.Fprintf(w, "failed %s: %s\n", failure.URL, failure.Message); err != nil {
			return err
		}
	}
	return nil
}

func printIllusts(w io.Writer, illusts []sdk.Illust, offset int, ranked bool) error {
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

func printIllust(w io.Writer, illust sdk.Illust, rank int, compact bool) error {
	prefix := ""
	if rank > 0 {
		prefix = fmt.Sprintf("#%d ", rank)
	}
	tags := make([]string, 0, len(illust.Tags))
	for _, tag := range illust.Tags {
		tags = append(tags, tag.Name)
	}
	url := illust.URL
	if url == "" && illust.ID > 0 {
		url = fmt.Sprintf("https://www.pixiv.net/artworks/%d", illust.ID)
	}
	if compact {
		if _, err := fmt.Fprintf(w, "%s%s\n", prefix, url); err != nil {
			return err
		}
		_, err := fmt.Fprintf(w, "%d %q by %s bookmarks:%d views:%d tags:%s\n",
			illust.ID, illust.Title, illust.User.Name, illust.TotalBookmarks, illust.TotalView, strings.Join(tags, ","))
		return err
	}
	for _, line := range []string{
		fmt.Sprintf("url: %s\n", url), fmt.Sprintf("id: %d\n", illust.ID), fmt.Sprintf("title: %s\n", illust.Title),
		fmt.Sprintf("author: %s (%d)\n", illust.User.Name, illust.User.ID), fmt.Sprintf("type: %s\n", illust.Type),
		fmt.Sprintf("page_count: %d\n", illust.PageCount), fmt.Sprintf("bookmarks: %d\n", illust.TotalBookmarks),
		fmt.Sprintf("views: %d\n", illust.TotalView), fmt.Sprintf("tags: %s\n", strings.Join(tags, ",")),
	} {
		if _, err := io.WriteString(w, line); err != nil {
			return err
		}
	}
	if caption := captionPlainText(illust.Caption); caption != "" {
		if _, err := fmt.Fprintf(w, "caption:\n%s\n", caption); err != nil {
			return err
		}
	}
	return nil
}
