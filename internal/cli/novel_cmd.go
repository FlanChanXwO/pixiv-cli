package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
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

func (a app) newNovelCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "novel", Short: "Query Pixiv novels"}
	cmd.AddCommand(a.newNovelSearchCommand())
	return cmd
}

func (a app) newNovelSearchCommand() *cobra.Command {
	opts := novelSearchOptions{
		searchBy: searchTargetTagPartial,
		sortMode: string(pixiv.SortModeDateDesc),
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
	bindNDJSONFlag(cmd, &opts.ndjsonOutputOptions)
	flags := cmd.Flags()
	flags.StringVar(&opts.searchBy, "search-by", opts.searchBy, "search field: tag-partial, tag-exact, title-caption")
	flags.StringVar(&opts.sortMode, "sort", opts.sortMode, "sort mode: date_desc, date_asc")
	flags.StringVar(&opts.period, "period", "", "time range: day, week, month")
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
		return newUsageError(fmt.Errorf("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !opts.ndjson {
		jsonOut, err = services.SDK.JSONOut(jsonOverride)
		if err != nil {
			return err
		}
	}
	word := strings.Join(args, " ")
	fetch := func(client application.SDKClient, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
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

func (a app) runNovelListNDJSON(ctx context.Context, plan listPlan, fetch func(context.Context, sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error)) error {
	encoder := json.NewEncoder(a.out)
	return pageItems(ctx, plan, fetch, func(items []pixiv.Novel) error {
		return encodeNDJSONRecords(encoder, items, application.RecordFromNovel)
	})
}

func (a app) runNovelList(ctx context.Context, plan listPlan, jsonOut bool, fetch func(context.Context, sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error), print func([]pixiv.Novel) error) error {
	if jsonOut {
		spool, err := newJSONArraySpool("novels")
		if err != nil {
			return err
		}
		defer spool.Close()
		if err := pageItems(ctx, plan, fetch, func(items []pixiv.Novel) error { return appendJSONArray(spool, items) }); err != nil {
			return err
		}
		return spool.Commit(a.out)
	}
	return pageItems(ctx, plan, fetch, print)
}
