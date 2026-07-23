package cli

import (
	"context"
	"fmt"
	"strings"

	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/spf13/cobra"
)

type novelSearchOptions struct {
	commandOptions
	listOptions
	searchBy      string
	sortMode      string
	period        string
	rating        string
	minTextLength int
	maxTextLength int
	originalOnly  bool
}

func (a app) newNovelCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "novel", Short: "Query Pixiv novels"}
	cmd.AddCommand(a.newNovelSearchCommand())
	return cmd
}

func (a app) newNovelSearchCommand() *cobra.Command {
	opts := novelSearchOptions{
		searchBy: searchTargetTagPartial,
		sortMode: string(sdk.SortModeDateDesc),
		rating:   string(sdk.SearchRatingAll),
	}
	cmd := &cobra.Command{
		Use:     "search WORD",
		Short:   "Search novels",
		Example: "pixiv novel search \"miku\" --json",
		Args:    requireMinArgs(1, "pixiv novel search [options] WORD"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runNovelSearch(cmd, args, opts)
		},
	}
	a.bindCommonFlags(cmd, &opts.commandOptions)
	flags := cmd.Flags()
	flags.StringVar(&opts.searchBy, "search-by", opts.searchBy, "search field: tag-partial, tag-exact, title-caption")
	flags.StringVar(&opts.sortMode, "sort", opts.sortMode, "sort mode: date_desc, date_asc")
	flags.StringVar(&opts.period, "period", "", "time range: day, week, month")
	flags.StringVar(&opts.rating, "rating", opts.rating, "rating filter: sfw, r18, r18g, mature, all")
	flags.IntVar(&opts.minTextLength, "min-text-length", 0, "minimum text length in characters; 0 disables the bound")
	flags.IntVar(&opts.maxTextLength, "max-text-length", 0, "maximum text length in characters; 0 disables the bound")
	flags.BoolVar(&opts.originalOnly, "original-only", false, "only original novels")
	bindListFlags(cmd, &opts.listOptions)
	return cmd
}

func (a app) runNovelSearch(cmd *cobra.Command, args []string, opts novelSearchOptions) error {
	target, err := resolveSearchBy(opts.searchBy)
	if err != nil {
		return err
	}
	period, err := resolveSearchPeriod(opts.period)
	if err != nil {
		return err
	}
	filters, err := resolveNovelSearchFilters(opts)
	if err != nil {
		return err
	}
	plan, err := parseListPlan(cmd, opts.listOptions)
	if err != nil {
		return err
	}
	request, jsonOverride, err := a.sdkRequest(cmd, opts.commandOptions)
	if err != nil {
		return err
	}
	services := a.services()
	jsonOut, err := services.SDK.JSONOut(jsonOverride)
	if err != nil {
		return err
	}
	client, err := services.SDK.OpenOperation(cmd.Context(), request)
	if err != nil {
		return err
	}
	word := strings.Join(args, " ")
	if !jsonOut {
		fmt.Fprintf(a.out, "novels for %q\n", word)
	}
	return a.runNovelList(cmd.Context(), plan, jsonOut, func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Novel, sdk.Cursor, error) {
		result, searchErr := client.SearchNovel(ctx, sdk.SearchNovelRequest{
			Word: word, Target: target, Sort: sdk.SortMode(opts.sortMode), Duration: period, Cursor: cursor, Filters: filters,
		})
		if searchErr != nil {
			return nil, "", searchErr
		}
		return result.Novels, result.NextCursor, nil
	}, func(items []sdk.Novel) { printNovels(a.out, items) })
}

func resolveNovelSearchFilters(opts novelSearchOptions) (sdk.NovelSearchFilters, error) {
	filters := sdk.NovelSearchFilters{MinTextLength: opts.minTextLength, MaxTextLength: opts.maxTextLength, OriginalOnly: opts.originalOnly}
	switch opts.rating {
	case "sfw", "r18", "r18g", "mature", "all":
		filters.Rating = sdk.SearchRating(opts.rating)
	default:
		return filters, fmt.Errorf("rating must be one of sfw, r18, r18g, mature, all")
	}
	if filters.MinTextLength < 0 || filters.MaxTextLength < 0 {
		return filters, fmt.Errorf("text length bounds must be zero or positive integers")
	}
	if filters.MaxTextLength > 0 && filters.MinTextLength > filters.MaxTextLength {
		return filters, fmt.Errorf("min-text-length cannot exceed max-text-length")
	}
	return filters, nil
}

func (a app) runNovelList(ctx context.Context, plan listPlan, jsonOut bool, fetch func(context.Context, sdk.Cursor) ([]sdk.Novel, sdk.Cursor, error), print func([]sdk.Novel)) error {
	if jsonOut {
		spool, err := newJSONArraySpool("novels")
		if err != nil {
			return err
		}
		defer spool.Close()
		if err := pageItems(ctx, plan, fetch, func(items []sdk.Novel) error { return appendJSONArray(spool, items) }); err != nil {
			return err
		}
		return spool.Commit(a.out)
	}
	return pageItems(ctx, plan, fetch, func(items []sdk.Novel) error {
		print(items)
		return nil
	})
}
