// Package ranking 提供 pixiv ranking 命令，读取 Pixiv 作品与小说排行。
package ranking

import (
	"context"
	"errors"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/display"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/target"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

type options struct {
	environment.Options
	ndjson bool
	typ    string
	mode   string
	date   string
	limit  int
	page   int
}

type commandRunner struct {
	env    environment.Env
	runner session.Runner
}

// New 构造 `pixiv ranking` 命令，作品和小说排行共用同一套分页与输出。
func New(env environment.Env, exec session.Executor) *cobra.Command {
	a := commandRunner{env: env, runner: session.NewRunner(env.Out, exec)}
	options := options{typ: string(target.NamespaceArtwork), mode: string(pixiv.RankingModeDay)}
	cmd := &cobra.Command{
		Use:   "ranking",
		Short: "Show artwork or novel ranking",
		Args:  env.ExactArgs(0, "pixiv ranking [options]"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.run(cmd, options)
		},
	}
	env.BindCommonFlags(cmd, &options.Options)
	environment.BindNDJSONFlag(cmd, &options.ndjson)
	flags := cmd.Flags()
	flags.StringVarP(&options.typ, "type", "t", options.typ, "entity type: artwork or novel")
	flags.StringVar(&options.mode, "mode", options.mode, "ranking mode: day, day_male, day_female, week, week_original, week_rookie, month, day_manga, week_manga, month_manga, week_rookie_manga, day_r18, day_male_r18, day_female_r18, week_r18, week_r18g; the last nine require authentication")
	flags.StringVar(&options.date, "date", "", "YYYY-MM-DD (artwork ranking only)")
	environment.BindListFlags(cmd, &options.limit, &options.page)
	env.BindNoInput(cmd)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a commandRunner) run(cmd *cobra.Command, opts options) error {
	if opts.typ != string(target.NamespaceArtwork) && opts.typ != string(target.NamespaceNovel) {
		return errors.New("type must be one of: artwork, novel")
	}
	if opts.typ == string(target.NamespaceNovel) && cmd.Flags().Changed("date") {
		return errors.New("--date is only supported when --type artwork")
	}
	switch pixiv.RankingMode(opts.mode) {
	case "", pixiv.RankingModeDay, pixiv.RankingModeDayMale, pixiv.RankingModeDayFemale, pixiv.RankingModeWeek,
		pixiv.RankingModeWeekOriginal, pixiv.RankingModeWeekRookie, pixiv.RankingModeMonth,
		pixiv.RankingModeDayManga, pixiv.RankingModeWeekManga, pixiv.RankingModeMonthManga,
		pixiv.RankingModeWeekRookieManga, pixiv.RankingModeDayR18, pixiv.RankingModeDayMaleR18,
		pixiv.RankingModeDayFemaleR18, pixiv.RankingModeWeekR18, pixiv.RankingModeWeekR18G:
	default:
		return errors.New("mode must be one of: day, day_male, day_female, week, week_original, week_rookie, month, day_manga, week_manga, month_manga, week_rookie_manga, day_r18, day_male_r18, day_female_r18, week_r18, week_r18g; see pixiv ranking --help")
	}
	plan, err := environment.ParsePlan(cmd, opts.limit, opts.page)
	if err != nil {
		return err
	}
	request, err := a.env.Request(cmd, opts.Options.ProxyOptions)
	if err != nil {
		return err
	}
	if opts.ndjson && cmd.Flags().Changed("json") {
		return a.env.WrapUsage(errors.New("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !opts.ndjson {
		jsonOut, err = a.env.ResolveJSON(a.env.JSONOverride(cmd, opts.Options))
		if err != nil {
			return err
		}
	}
	ndjson := a.env.AutoNDJSON(cmd, opts.ndjson, jsonOut)
	format := record.FormatOf(jsonOut, ndjson)
	if opts.typ == string(target.NamespaceNovel) {
		fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
			result, err := client.NovelRanking(ctx, pixiv.NovelRankingRequest{Mode: pixiv.RankingMode(opts.mode), Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			return result.Items, result.Next, nil
		}
		position := plan.Skip
		return record.RunNovels(cmd.Context(), a.runner, request, plan, format, a.env.Err, record.NovelList{
			Fetch: fetch,
			Print: func(items []pixiv.Novel) error {
				for _, item := range items {
					position++
					if _, err := fmt.Fprintf(a.env.Out, "#%d ", position); err != nil {
						return err
					}
					if err := display.PrintNovels(a.env.Out, []pixiv.Novel{item}); err != nil {
						return err
					}
				}
				return nil
			},
		})
	}
	fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		result, err := client.ArtworkRanking(ctx, pixiv.ArtworkRankingRequest{Mode: pixiv.RankingMode(opts.mode), Date: opts.date, Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return record.RunArtworks(cmd.Context(), a.runner, request, plan, format, a.env.Err, record.ArtworkList{
		Fetch: fetch,
		Print: func(items []pixiv.Artwork, start int) error {
			return display.PrintArtworksRanked(a.env.Out, items, start)
		},
	})
}
