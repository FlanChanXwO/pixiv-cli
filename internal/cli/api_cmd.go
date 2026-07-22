package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/parse"
	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/spf13/cobra"
)

type searchOptions struct {
	commandOptions
	searchBy    string
	sortMode    string
	period      string
	resolution  string
	aspectRatio string
	drawTool    string
	aiMode      string
	listOptions
	rating string
	typ    string
}

type rankingOptions struct {
	commandOptions
	mode string
	date string
	listOptions
}

type recommendedOptions struct {
	commandOptions
	listOptions
}

const (
	searchTargetTagPartial   = "tag-partial"
	searchTargetTagExact     = "tag-exact"
	searchTargetTitleCaption = "title-caption"
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
	flags := cmd.Flags()
	flags.StringVar(&opts.searchBy, "search-by", opts.searchBy, "search field: tag-partial, tag-exact, title-caption")
	flags.StringVar(&opts.sortMode, "sort", opts.sortMode, "sort mode: date_desc, date_asc")
	flags.StringVar(&opts.period, "period", "", "time range: day, week, month")
	flags.StringVar(&opts.rating, "rating", opts.rating, "rating filter: sfw, r18, r18g, mature, all")
	flags.StringVar(&opts.typ, "type", opts.typ, "artwork type filter: all, illust-and-ugoira, illust, manga, ugoira")
	flags.StringVar(&opts.resolution, "resolution", opts.resolution, "resolution filter: all, high, medium, low")
	flags.StringVar(&opts.aspectRatio, "aspect-ratio", opts.aspectRatio, "aspect ratio filter: all, landscape, portrait, square")
	flags.StringVar(&opts.drawTool, "draw-tool", "", "drawing tool name from search-options")
	flags.StringVar(&opts.aiMode, "ai-mode", opts.aiMode, "AI artwork filter: all, exclude, only")
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
	jsonOut, err := services.SDK.JSONOut(jsonOverride)
	if err != nil {
		return err
	}
	client, err := services.SDK.OpenOperation(cmd.Context(), clientReq)
	if err != nil {
		return err
	}
	if !jsonOut {
		fmt.Fprintf(a.out, "illustrations for %q\n", word)
	}
	return a.runIllustList(cmd.Context(), plan, jsonOut, func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
		result, err := client.SearchIllust(ctx, sdk.SearchIllustRequest{
			Word: word, Target: target, Sort: sdk.SortMode(opts.sortMode),
			Duration: period, Cursor: cursor, Filters: filters,
		})
		if err != nil {
			return nil, "", err
		}
		return result.Illusts, result.NextCursor, nil
	}, func(items []sdk.Illust, start int) { printIllusts(a.out, items, start, false) })
}

func resolveSearchBy(value string) (sdk.SearchTarget, error) {
	switch value {
	case searchTargetTagPartial:
		return sdk.SearchTargetPartialMatchForTags, nil
	case searchTargetTagExact:
		return sdk.SearchTargetExactMatchForTags, nil
	case searchTargetTitleCaption:
		return sdk.SearchTargetTitleAndCaption, nil
	default:
		return "", fmt.Errorf("search-by must be one of %s, %s, %s", searchTargetTagPartial, searchTargetTagExact, searchTargetTitleCaption)
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
	default:
		return "", errors.New("period must be one of day, week, month")
	}
}

func resolveSearchFilters(_ *cobra.Command, opts searchOptions) (sdk.SearchIllustFilters, error) {
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
	client, err := services.SDK.OpenOperation(cmd.Context(), clientReq)
	if err != nil {
		return err
	}
	result, err := client.SearchIllustOptions(cmd.Context(), sdk.SearchIllustOptionsRequest{Word: word})
	if err != nil {
		return err
	}
	if jsonOut {
		return a.printJSON(result)
	}
	fmt.Fprintf(a.out, "search options for %q\n", word)
	if len(result.Tools) == 0 {
		fmt.Fprintln(a.out, "tools: none")
		return nil
	}
	fmt.Fprintln(a.out, "tools:")
	for _, tool := range result.Tools {
		fmt.Fprintf(a.out, "- %s\n", safeTextLine(tool))
	}
	return nil
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
		Use:   "detail ILLUST_ID",
		Short: "Show one illustration",
		Args:  requireExactArgs(1, "pixiv detail [options] ILLUST_ID"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runDetail(cmd, args[0], opts)
		},
	}
	a.bindCommonFlags(cmd, &opts)
	return cmd
}

func (a app) runDetail(cmd *cobra.Command, arg string, opts commandOptions) error {
	id, err := parse.PositiveInt64(arg, "illust_id")
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
	client, err := services.SDK.OpenOperation(cmd.Context(), clientReq)
	if err != nil {
		return err
	}
	result, err := client.IllustDetail(cmd.Context(), id)
	if err != nil {
		return err
	}
	if jsonOut {
		return a.printJSON(result)
	}
	printIllust(a.out, result.Illust, 0, false)
	return nil
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
	jsonOut, err := services.SDK.JSONOut(jsonOverride)
	if err != nil {
		return err
	}
	client, err := services.SDK.OpenOperation(cmd.Context(), clientReq)
	if err != nil {
		return err
	}
	if !jsonOut {
		fmt.Fprintf(a.out, "%s ranking\n", opts.mode)
	}
	return a.runIllustList(cmd.Context(), plan, jsonOut, func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
		result, err := client.IllustRanking(ctx, sdk.IllustRankingRequest{Mode: sdk.RankingMode(opts.mode), Date: opts.date, Cursor: cursor})
		if err != nil {
			return nil, "", err
		}
		return result.Illusts, result.NextCursor, nil
	}, func(items []sdk.Illust, start int) { printIllusts(a.out, items, start, true) })
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
	bindListFlags(cmd, &opts.listOptions)
	return cmd
}

type downloadOptions struct {
	commandOptions
	downloadPath     string
	filenameTemplate string
	pages            string
	quality          string
}

func (a app) newDownloadCommand() *cobra.Command {
	opts := downloadOptions{quality: string(application.DownloadQualityOriginal)}
	cmd := &cobra.Command{
		Use:   "download ILLUST_ID...",
		Short: "Download illustrations",
		Args:  requireMinArgs(1, "pixiv download [options] ILLUST_ID..."),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runDownload(cmd, args, opts)
		},
	}
	a.bindCommonFlags(cmd, &opts.commandOptions)
	a.bindDownloadRuntimeFlags(cmd, &opts)
	flags := cmd.Flags()
	flags.StringVar(&opts.pages, "pages", "", "1-based page selection, e.g. 1,3-5; default all pages")
	flags.StringVar(&opts.quality, "quality", opts.quality, "static image quality: original, regular, small, thumb, mini")
	return cmd
}

// bindDownloadRuntimeFlags 只注册真正影响下载落盘的参数，避免非下载命令静默接受无效 flag。
func (a app) bindDownloadRuntimeFlags(cmd *cobra.Command, opts *downloadOptions) {
	flags := cmd.Flags()
	flags.StringVar(&opts.downloadPath, "download-path", "", "download directory")
	flags.StringVar(&opts.filenameTemplate, "filename-template", "", "filename template placeholders: {id}, {title}, {author}")
}

func (a app) runDownload(cmd *cobra.Command, args []string, opts downloadOptions) error {
	ids := make([]int64, 0, len(args))
	for _, arg := range args {
		id, err := parse.PositiveInt64(arg, fmt.Sprintf("illust_id %q", arg))
		if err != nil {
			return err
		}
		ids = append(ids, id)
	}
	pages, err := application.ParsePageSpec(opts.pages)
	if err != nil {
		return err
	}
	quality := application.DownloadQuality(opts.quality)
	if quality == "" {
		quality = application.DownloadQualityOriginal
	}
	if err := application.ValidateDownloadQuality(quality); err != nil {
		return err
	}
	services := a.services()
	clientReq, err := a.clientRequest(cmd, opts.commandOptions, false)
	if err != nil {
		return err
	}
	jsonOut, err := services.SDK.JSONOut(clientReq.JSONOverride)
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
	client, err := services.SDK.OpenOperation(cmd.Context(), application.SDKClientRequest{
		UserID: clientReq.UserID, RefreshToken: clientReq.RefreshToken, HTTPSProxyOverride: clientReq.HTTPSProxyOverride,
	})
	if err != nil {
		return err
	}
	artworks, err := services.Download.Download(cmd.Context(), client, application.DownloadRequest{
		IllustIDs:        ids,
		DownloadPath:     runtime.DownloadPath,
		FilenameTemplate: runtime.FilenameTemplate,
		Pages:            pages,
		Quality:          quality,
	})
	if err != nil {
		return err
	}
	if jsonOut {
		return a.printJSON(artworks)
	}
	for _, artwork := range artworks {
		fmt.Fprintf(a.out, "downloaded %d %q by %s\n", artwork.IllustID, artwork.Title, artwork.Author)
		for _, file := range artwork.Files {
			fmt.Fprintf(a.out, "  %s\n", file.Path)
		}
	}
	return nil
}

func printIllusts(w io.Writer, illusts []sdk.Illust, offset int, ranked bool) {
	for i, illust := range illusts {
		rank := 0
		if ranked {
			rank = i + 1 + offset
		}
		printIllust(w, illust, rank, true)
	}
}

func printIllust(w io.Writer, illust sdk.Illust, rank int, compact bool) {
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
		fmt.Fprintf(w, "%s%s\n", prefix, url)
		fmt.Fprintf(w, "%d %q by %s bookmarks:%d views:%d tags:%s\n",
			illust.ID, illust.Title, illust.User.Name, illust.TotalBookmarks, illust.TotalView, strings.Join(tags, ","))
		return
	}
	fmt.Fprintf(w, "url: %s\n", url)
	fmt.Fprintf(w, "id: %d\n", illust.ID)
	fmt.Fprintf(w, "title: %s\n", illust.Title)
	fmt.Fprintf(w, "author: %s (%d)\n", illust.User.Name, illust.User.ID)
	fmt.Fprintf(w, "type: %s\n", illust.Type)
	fmt.Fprintf(w, "page_count: %d\n", illust.PageCount)
	fmt.Fprintf(w, "bookmarks: %d\n", illust.TotalBookmarks)
	fmt.Fprintf(w, "views: %d\n", illust.TotalView)
	fmt.Fprintf(w, "tags: %s\n", strings.Join(tags, ","))
}
