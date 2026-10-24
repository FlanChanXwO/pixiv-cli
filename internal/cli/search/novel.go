package search

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	pixivcmd "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivcmd"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

type novelOptions struct {
	command.Options
	ndjson   bool
	limit    int
	page     int
	searchBy string
	sortMode string
	period   string
}

// NewNovel builds the canonical `pixiv novel` compatibility group. Its search
// leaf shares this package's execution with `pixiv search --type novel`.
func NewNovel(env command.Env, exec pixivcmd.Executor, reverseSearch ReverseSearchPort) *cobra.Command {
	a := commandRunner{env: env, runner: pixivcmd.New(env.Out, exec), exec: exec, reverseSearch: reverseSearch}
	cmd := &cobra.Command{Use: "novel", Short: "Query Pixiv novels"}
	cmd.AddCommand(a.newNovelSearchCommand())
	a.env.BindNoInput(cmd)
	command.Bind(cmd, command.PixivData())
	return cmd
}

func (a commandRunner) newNovelSearchCommand() *cobra.Command {
	opts := novelOptions{
		searchBy: searchTargetTagPartial,
		sortMode: string(pixiv.SortModeDateDesc),
	}
	cmd := &cobra.Command{
		Use:     "search WORD",
		Short:   "Search novels",
		Example: "pixiv novel search \"miku\" --json",
		Args:    a.env.MinArgs(1, "pixiv novel search [options] WORD"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runNovelSearch(cmd, args, opts)
		},
	}
	a.env.BindCommonFlags(cmd, &opts.Options)
	pixivcmd.BindNDJSONFlag(cmd, &opts.ndjson)
	flags := cmd.Flags()
	flags.StringVar(&opts.searchBy, "search-by", opts.searchBy, "search field: tag-partial, tag-exact, title-caption")
	flags.StringVar(&opts.sortMode, "sort", opts.sortMode, "sort mode: date_desc, date_asc")
	flags.StringVar(&opts.period, "period", "", "time range: day, week, month")
	pixivcmd.BindListFlags(cmd, &opts.limit, &opts.page)
	a.env.BindTextValue(cmd, 1, -1, 0)
	command.Bind(cmd, command.PixivData())
	return cmd
}

func (a commandRunner) runNovelSearch(cmd *cobra.Command, args []string, opts novelOptions) error {
	target, err := resolveNovelSearchBy(opts.searchBy)
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
	plan, err := pixivcmd.ParsePlan(cmd, opts.limit, opts.page)
	if err != nil {
		return err
	}
	request, err := pixivcmd.BuildRequest(cmd, opts.Options)
	if err != nil {
		return err
	}
	jsonOverride := a.env.JSONOverride(cmd, opts.Options)
	if opts.ndjson && cmd.Flags().Changed("json") {
		return a.env.UsageError(errors.New("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !opts.ndjson {
		jsonOut, err = a.env.JSONOut(jsonOverride)
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
	return a.runner.RunNovels(cmd.Context(), request, plan, pixivcmd.FormatOf(jsonOut, opts.ndjson), pixivcmd.NovelList{Heading: fmt.Sprintf("novels for %q", word), Fetch: fetch, Print: func(items []pixiv.Novel) error { return pixivcmd.PrintNovels(a.env.Out, items) }})
}

// resolveNovelSearchBy 将小说搜索 target 限定为 v1 合同支持的三种值，避免复用
// artwork 的 tag-title-caption 能力后把不支持的 keyword 请求发给 upstream。
func resolveNovelSearchBy(value string) (pixiv.SearchTarget, error) {
	switch value {
	case searchTargetTagPartial:
		return pixiv.SearchTargetPartialMatchForTags, nil
	case searchTargetTagExact:
		return pixiv.SearchTargetExactMatchForTags, nil
	case searchTargetTitleCaption:
		return pixiv.SearchTargetTitleAndCaption, nil
	default:
		return "", fmt.Errorf("search-by must be one of %s, %s, %s", searchTargetTagPartial, searchTargetTagExact, searchTargetTitleCaption)
	}
}
