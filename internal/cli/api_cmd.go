package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/utils/parse"
	sdk "github.com/FlanChanXwO/pixiv-cli/pkg/pixiv"
	"github.com/spf13/cobra"
)

type searchOptions struct {
	commandOptions
	target   string
	sortMode string
	duration string
	listOptions
	r18    bool
	rating string
	typ    string
	aiType int
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

func (a app) newSearchCommand() *cobra.Command {
	opts := searchOptions{
		target:   string(sdk.SearchTargetPartialMatchForTags),
		sortMode: string(sdk.SortModeDateDesc),
		rating:   "all",
		typ:      "all",
		aiType:   2,
	}
	cmd := &cobra.Command{
		Use:     "search WORD",
		Short:   "Search illustrations",
		Example: "pixiv search \"初音ミク\" --json",
		Args:    requireMinArgs(1, "pixiv search [options] WORD"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runSearch(cmd, args, opts)
		},
	}
	a.bindCommonFlags(cmd, &opts.commandOptions)
	flags := cmd.Flags()
	flags.StringVar(&opts.target, "search-target", opts.target, "search target")
	flags.StringVar(&opts.sortMode, "sort", opts.sortMode, "sort mode")
	flags.StringVar(&opts.duration, "duration", "", "duration")
	flags.StringVar(&opts.rating, "rating", opts.rating, "rating filter: sfw, r18, r18g, mature, all")
	flags.StringVar(&opts.typ, "type", opts.typ, "artwork type filter: illust, comics, ugoira, all")
	flags.IntVar(&opts.aiType, "ai-type", opts.aiType, "AI artwork filter: 0 non-AI, 1 AI only, 2 all")
	bindListFlags(cmd, &opts.listOptions)
	flags.BoolVar(&opts.r18, "r18", false, "append R-18 to the search word")
	return cmd
}

func (a app) runSearch(cmd *cobra.Command, args []string, opts searchOptions) error {
	if err := validateSearchFilters(opts); err != nil {
		return err
	}
	word := strings.Join(args, " ")
	if opts.r18 {
		word += " R-18"
	}
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
		result, err := client.SearchIllust(ctx, sdk.SearchIllustRequest{Word: word, Target: sdk.SearchTarget(opts.target), Sort: sdk.SortMode(opts.sortMode), Duration: opts.duration, Cursor: cursor})
		if err != nil {
			return nil, "", err
		}
		return filterSearchIllusts(result.Illusts, opts), result.NextCursor, nil
	}, func(items []sdk.Illust, start int) { printIllusts(a.out, items, start, false) })
}

func validateSearchFilters(opts searchOptions) error {
	switch opts.rating {
	case "sfw", "r18", "r18g", "mature", "all":
	default:
		return fmt.Errorf("rating must be one of sfw, r18, r18g, mature, all")
	}
	switch opts.typ {
	case "illust", "comics", "ugoira", "all":
	default:
		return fmt.Errorf("type must be one of illust, comics, ugoira, all")
	}
	if opts.aiType < 0 || opts.aiType > 2 {
		return fmt.Errorf("ai-type must be 0, 1, or 2")
	}
	return nil
}

// filterSearchIllusts 保持上游原始 cursor 不变，仅在 CLI 输出边界筛选结果。
// 因此 --limit/--page 仍由通用分页器按筛选后的逻辑结果计数，并会在未满足数量时继续取下一批。
func filterSearchIllusts(illusts []sdk.Illust, opts searchOptions) []sdk.Illust {
	filtered := make([]sdk.Illust, 0, len(illusts))
	for _, illust := range illusts {
		if !matchesSearchRating(illust.XRestrict, opts.rating) || !matchesSearchType(illust.Type, opts.typ) || !matchesSearchAIType(illust.AIType, opts.aiType) {
			continue
		}
		filtered = append(filtered, illust)
	}
	return filtered
}

func matchesSearchRating(xRestrict int, rating string) bool {
	switch rating {
	case "sfw":
		return xRestrict == 0
	case "r18":
		return xRestrict == 1
	case "r18g":
		return xRestrict == 2
	case "mature":
		return xRestrict == 0 || xRestrict == 1
	case "all":
		return true
	default:
		return false
	}
}

func matchesSearchType(actual, want string) bool {
	if want == "all" {
		return true
	}
	if want == "comics" {
		return actual == string(sdk.IllustTypeManga)
	}
	return actual == want
}

func matchesSearchAIType(actual, want int) bool {
	switch want {
	case 0, 1:
		return actual == want
	case 2:
		return true
	default:
		return false
	}
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
	flags.StringVar(&opts.mode, "mode", opts.mode, "ranking mode")
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
		Use:   "recommended",
		Short: "Show personalized recommendations",
		Args:  requireExactArgs(0, "pixiv recommended [options]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runRecommended(cmd, opts)
		},
	}
	a.bindCommonFlags(cmd, &opts.commandOptions)
	bindListFlags(cmd, &opts.listOptions)
	return cmd
}

func (a app) runRecommended(cmd *cobra.Command, opts recommendedOptions) error {
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
		fmt.Fprintln(a.out, "recommended illustrations")
	}
	return a.runIllustList(cmd.Context(), plan, jsonOut, func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
		result, err := client.IllustRecommended(ctx, sdk.IllustRecommendedRequest{Cursor: cursor})
		if err != nil {
			return nil, "", err
		}
		return result.Illusts, result.NextCursor, nil
	}, func(items []sdk.Illust, start int) { printIllusts(a.out, items, start, false) })
}

func (a app) newDownloadCommand() *cobra.Command {
	var opts commandOptions
	cmd := &cobra.Command{
		Use:   "download ILLUST_ID...",
		Short: "Download illustrations",
		Args:  requireMinArgs(1, "pixiv download [options] ILLUST_ID..."),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runDownload(cmd, args, opts)
		},
	}
	a.bindCommonFlags(cmd, &opts)
	return cmd
}

func (a app) runDownload(cmd *cobra.Command, args []string, opts commandOptions) error {
	ids := make([]int64, 0, len(args))
	for _, arg := range args {
		id, err := parse.PositiveInt64(arg, fmt.Sprintf("illust_id %q", arg))
		if err != nil {
			return err
		}
		ids = append(ids, id)
	}
	services := a.services()
	clientReq, err := a.clientRequest(cmd, opts, false)
	if err != nil {
		return err
	}
	artworks, jsonOut, err := services.Download.Download(cmd.Context(), clientReq, ids)
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
	if compact {
		fmt.Fprintf(w, "%s%d %q by %s bookmarks:%d views:%d tags:%s\n",
			prefix, illust.ID, illust.Title, illust.User.Name, illust.TotalBookmarks, illust.TotalView, strings.Join(tags, ","))
		return
	}
	fmt.Fprintf(w, "id: %d\n", illust.ID)
	fmt.Fprintf(w, "title: %s\n", illust.Title)
	fmt.Fprintf(w, "author: %s (%d)\n", illust.User.Name, illust.User.ID)
	fmt.Fprintf(w, "type: %s\n", illust.Type)
	fmt.Fprintf(w, "page_count: %d\n", illust.PageCount)
	fmt.Fprintf(w, "bookmarks: %d\n", illust.TotalBookmarks)
	fmt.Fprintf(w, "views: %d\n", illust.TotalView)
	fmt.Fprintf(w, "tags: %s\n", strings.Join(tags, ","))
}
