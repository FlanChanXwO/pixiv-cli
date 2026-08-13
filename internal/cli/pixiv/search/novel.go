package search

import (
	"context"
	"errors"
	"fmt"
	"strings"

	deps "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivdeps"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pixiv/internal/listing"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/requirements"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

type novelOptions struct {
	deps.CommandOptions
	ndjson   bool
	limit    int
	page     int
	searchBy string
	sortMode string
	period   string
}

// NewNovel builds the canonical `pixiv novel` compatibility group. Its search
// leaf shares this package's execution with `pixiv search --type novel`.
func NewNovel(data deps.Data) *cobra.Command {
	a := command{data: data}
	cmd := &cobra.Command{Use: "novel", Short: "Query Pixiv novels"}
	cmd.AddCommand(a.newNovelSearchCommand())
	data.BindNoInput(cmd)
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

func (a command) newNovelSearchCommand() *cobra.Command {
	opts := novelOptions{
		searchBy: searchTargetTagPartial,
		sortMode: string(pixiv.SortModeDateDesc),
	}
	cmd := &cobra.Command{
		Use:     "search WORD",
		Short:   "Search novels",
		Example: "pixiv novel search \"miku\" --json",
		Args:    a.data.MinArgs(1, "pixiv novel search [options] WORD"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runNovelSearch(cmd, args, opts)
		},
	}
	a.data.BindCommonFlags(cmd, &opts.CommandOptions)
	listing.BindNDJSONFlag(cmd, &opts.ndjson)
	flags := cmd.Flags()
	flags.StringVar(&opts.searchBy, "search-by", opts.searchBy, "search field: tag-partial, tag-exact, title-caption")
	flags.StringVar(&opts.sortMode, "sort", opts.sortMode, "sort mode: date_desc, date_asc")
	flags.StringVar(&opts.period, "period", "", "time range: day, week, month")
	listing.BindListFlags(cmd, &opts.limit, &opts.page)
	a.data.BindTextValue(cmd, 1, -1, 0)
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

func (a command) runNovelSearch(cmd *cobra.Command, args []string, opts novelOptions) error {
	target, err := resolveSearchBy(opts.searchBy)
	if err != nil {
		return err
	}
	period, err := resolvePeriod(opts.period)
	if err != nil {
		return err
	}
	if opts.period == "half-year" || opts.period == "year" {
		return errors.New("novel period must be one of day, week, month")
	}
	plan, err := listing.ParsePlan(cmd, opts.limit, opts.page)
	if err != nil {
		return err
	}
	request, err := a.data.Request(cmd, opts.CommandOptions)
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
	word := strings.Join(args, " ")
	fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
		result, searchErr := client.SearchNovels(ctx, pixiv.SearchNovelsRequest{
			Word: word, Target: target, Sort: pixiv.SortMode(opts.sortMode), Duration: period, Cursor: cursor,
		})
		if searchErr != nil {
			return nil, sdk.Cursor{}, searchErr
		}
		return result.Items, result.Next, nil
	}
	return a.runner().RunPooledNovelList(cmd.Context(), request, plan, jsonOut, opts.ndjson, fmt.Sprintf("novels for %q", word), fetch,
		func(items []pixiv.Novel) error { return printNovels(a.data.Output, items) })
}
