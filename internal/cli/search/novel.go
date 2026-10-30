package search

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/display"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

type novelOptions struct {
	environment.Options
	ndjson   bool
	limit    int
	page     int
	searchBy string
	sortMode string
	period   string
}

// NewNovel 构造 `pixiv novel` 命令组，其 search 叶子与 `pixiv search --type novel`
// 共用同一套执行逻辑。
func NewNovel(env environment.Env, exec session.Executor, reverseSearch ReverseSearchPort) *cobra.Command {
	a := commandRunner{env: env, runner: session.NewRunner(env.Out, exec), exec: exec, reverseSearch: reverseSearch}
	cmd := &cobra.Command{Use: "novel", Short: "Query Pixiv novels"}
	cmd.AddCommand(a.newNovelSearchCommand())
	a.env.BindNoInput(cmd)
	environment.Bind(cmd, environment.Normal())
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
	environment.BindNDJSONFlag(cmd, &opts.ndjson)
	flags := cmd.Flags()
	flags.StringVar(&opts.searchBy, "search-by", opts.searchBy, "search field: tag-partial, tag-exact, title-caption")
	flags.StringVar(&opts.sortMode, "sort", opts.sortMode, "sort mode: date_desc, date_asc, popular_desc")
	flags.StringVar(&opts.period, "period", "", "time range: day, week, month")
	environment.BindListFlags(cmd, &opts.limit, &opts.page)
	a.env.BindTextValue(cmd, 1, -1, 0)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a commandRunner) runNovelSearch(cmd *cobra.Command, args []string, opts novelOptions) error {
	if err := validateSort(opts.sortMode); err != nil {
		return err
	}
	target, err := resolveNovelSearchBy(opts.searchBy)
	if err != nil {
		return err
	}
	period, err := resolvePeriod(opts.period)
	if err != nil {
		return err
	}
	if opts.period == periodHalfYear || opts.period == periodYear {
		return errors.New("novel period must be one of day, week, month")
	}
	plan, err := environment.ParsePlan(cmd, opts.limit, opts.page)
	if err != nil {
		return err
	}
	request, err := a.env.Request(cmd, opts.Options.ProxyOptions)
	if err != nil {
		return err
	}
	jsonOverride := a.env.JSONOverride(cmd, opts.Options)
	if opts.ndjson && cmd.Flags().Changed("json") {
		return a.env.WrapUsage(errors.New("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !opts.ndjson {
		jsonOut, err = a.env.ResolveJSON(jsonOverride)
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
	return record.RunNovels(cmd.Context(), a.runner, request, plan, record.FormatOf(jsonOut, opts.ndjson), record.NovelList{Heading: fmt.Sprintf("novels for %q", word), Fetch: fetch, Print: func(items []pixiv.Novel) error { return display.PrintNovels(a.env.Out, items) }})
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
