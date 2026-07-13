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
	r18 bool
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
	bindListFlags(cmd, &opts.listOptions)
	flags.BoolVar(&opts.r18, "r18", false, "append R-18 to the search word")
	return cmd
}

func (a app) runSearch(cmd *cobra.Command, args []string, opts searchOptions) error {
	word := strings.Join(args, " ")
	if opts.r18 {
		word += " R-18"
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
	plan, err := parseListPlan(cmd, opts.listOptions)
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
		return result.Illusts, result.NextCursor, nil
	}, func(items []sdk.Illust, start int) { printIllusts(a.out, items, start, false) })
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
	plan, err := parseListPlan(cmd, opts.listOptions)
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
	plan, err := parseListPlan(cmd, opts.listOptions)
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
