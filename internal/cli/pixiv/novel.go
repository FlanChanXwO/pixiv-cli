package pixiv

import (
	"context"
	"fmt"
	"strings"

	pixivapp "github.com/FlanChanXwO/pixiv-cli/internal/application/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

type novelSearchOptions struct {
	commandOptions
	ndjsonOutputOptions
	listOptions
	searchBy string
	sortMode string
	period   string
}

func (a controller) newNovelCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "novel", Short: "Query Pixiv novels"}
	cmd.AddCommand(a.newNovelSearchCommand())
	return cmd
}

func (a controller) newNovelSearchCommand() *cobra.Command {
	opts := novelSearchOptions{
		searchBy: searchTargetTagPartial,
		sortMode: string(pixiv.SortModeDateDesc),
	}
	cmd := &cobra.Command{
		Use:     "search WORD",
		Short:   "Search novels",
		Example: "pixiv novel search \"miku\" --json",
		Args:    a.requireMinArgs(1, "pixiv novel search [options] WORD"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runNovelSearch(cmd, args, opts)
		},
	}
	a.bindCommonFlags(cmd, &opts.commandOptions)
	bindNDJSONFlag(cmd, &opts.ndjsonOutputOptions)
	flags := cmd.Flags()
	flags.StringVar(&opts.searchBy, "search-by", opts.searchBy, "search field: tag-partial, tag-exact, title-caption")
	flags.StringVar(&opts.sortMode, "sort", opts.sortMode, "sort mode: date_desc, date_asc")
	flags.StringVar(&opts.period, "period", "", "time range: day, week, month")
	bindListFlags(cmd, &opts.listOptions)
	return cmd
}

func (a controller) runNovelSearch(cmd *cobra.Command, args []string, opts novelSearchOptions) error {
	target, err := resolveSearchBy(opts.searchBy)
	if err != nil {
		return err
	}
	period, err := resolveSearchPeriod(opts.period)
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
	if opts.ndjson && cmd.Flags().Changed("json") {
		return a.usageError(fmt.Errorf("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !opts.ndjson {
		jsonOut, err = services.SDK.JSONOut(jsonOverride)
		if err != nil {
			return err
		}
	}
	word := strings.Join(args, " ")
	fetch := func(client pixivapp.ClientSet, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
		result, searchErr := client.SearchNovels(ctx, pixiv.SearchNovelsRequest{
			Word: word, Target: target, Sort: pixiv.SortMode(opts.sortMode), Duration: period, Cursor: cursor,
		})
		if searchErr != nil {
			return nil, sdk.Cursor{}, searchErr
		}
		return result.Items, result.Next, nil
	}
	return a.runPooledNovelList(cmd.Context(), request, plan, jsonOut, opts.ndjson, fmt.Sprintf("novels for %q", word), fetch, func(items []pixiv.Novel) error { return printNovels(a.out, items) })
}
