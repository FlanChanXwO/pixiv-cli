// Package ranking 提供 pixiv ranking 命令，读取 Pixiv 作品与小说排行。
package ranking

import (
	"context"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/display"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/target"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
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
	env  environment.Env
	exec environment.PixivExecutor
}

// New 构造 `pixiv ranking` 命令，作品和小说排行共用同一套分页与输出。
func New(env environment.Env, exec environment.PixivExecutor) *cobra.Command {
	a := commandRunner{env: env, exec: exec}
	options := options{typ: string(target.NamespaceArtwork), mode: string(pixiv.RankingModeDay)}
	cmd := &cobra.Command{
		Use:   "ranking",
		Short: "help.show_artwork_or_novel_ranking",
		Args:  env.ExactArgs(0, "pixiv ranking [options]"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.run(cmd, options)
		},
	}
	env.BindCommonFlags(cmd, &options.Options)
	environment.BindNDJSONFlag(cmd, &options.ndjson)
	flags := cmd.Flags()
	flags.StringVarP(&options.typ, "type", "t", options.typ, "flag.entity_type_artwork_or_novel")
	flags.StringVar(&options.mode, "mode", options.mode, "flag.ranking_mode_day_day_male_day_female_week")
	flags.StringVar(&options.date, "date", "", "flag.yyyy_mm_dd_artwork_ranking_only")
	environment.BindListFlags(cmd, &options.limit, &options.page)
	env.BindNoInput(cmd)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a commandRunner) run(cmd *cobra.Command, opts options) error {
	if opts.typ != string(target.NamespaceArtwork) && opts.typ != string(target.NamespaceNovel) {
		return i18n.Errorf("error.type_must_be_one_of_artwork_novel_333")
	}
	if opts.typ == string(target.NamespaceNovel) && cmd.Flags().Changed("date") {
		return i18n.Errorf("error.date_is_only_supported_when_type_artwork")
	}
	switch pixiv.RankingMode(opts.mode) {
	case "", pixiv.RankingModeDay, pixiv.RankingModeDayMale, pixiv.RankingModeDayFemale, pixiv.RankingModeWeek,
		pixiv.RankingModeWeekOriginal, pixiv.RankingModeWeekRookie, pixiv.RankingModeMonth,
		pixiv.RankingModeDayManga, pixiv.RankingModeWeekManga, pixiv.RankingModeMonthManga,
		pixiv.RankingModeWeekRookieManga, pixiv.RankingModeDayR18, pixiv.RankingModeDayMaleR18,
		pixiv.RankingModeDayFemaleR18, pixiv.RankingModeWeekR18, pixiv.RankingModeWeekR18G:
	default:
		return i18n.Errorf("error.mode_must_be_one_of_day_day_male")
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
		return a.env.WrapUsage(i18n.Errorf("error.ndjson_cannot_be_used_with_json"))
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
		fetch := func(ctx context.Context, _ int64, client *pixiv.Client, cursor sdk.Cursor) ([]record.Novel, sdk.Cursor, error) {
			result, err := client.NovelRanking(ctx, pixiv.NovelRankingOptions{Mode: pixiv.RankingMode(opts.mode), Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			items := make([]record.Novel, len(result.Items))
			for i, item := range result.Items {
				items[i], err = record.NovelOutput(item)
				if err != nil {
					return nil, sdk.Cursor{}, err
				}
			}
			return items, result.Cursor, nil
		}
		return record.RunSDKList(cmd.Context(), a.env.Out, a.env.Err, a.exec, request, plan, format, record.List[record.Novel]{
			RecordValue: func(item record.Novel) (any, error) { return record.RecordFromNovel(item) },
			Print: func(items []record.Novel, start int) error {
				for i, item := range items {
					position := start + i + 1
					if _, err := fmt.Fprintf(a.env.Out, "#%d ", position); err != nil {
						return err
					}
					if err := display.PrintNovels(a.env.Out, a.env.Language, []record.Novel{item}); err != nil {
						return err
					}
				}
				return nil
			},
		}, fetch)
	}
	fetch := func(ctx context.Context, _ int64, client *pixiv.Client, cursor sdk.Cursor) ([]record.Artwork, sdk.Cursor, error) {
		result, err := client.ArtworkRanking(ctx, pixiv.ArtworkRankingOptions{Mode: pixiv.RankingMode(opts.mode), Date: opts.date, Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		items := make([]record.Artwork, len(result.Items))
		for i, item := range result.Items {
			items[i], err = record.ArtworkOutput(item, false)
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
		}
		return items, result.Cursor, nil
	}
	return record.RunSDKList(cmd.Context(), a.env.Out, a.env.Err, a.exec, request, plan, format, record.List[record.Artwork]{
		RecordValue: func(item record.Artwork) (any, error) { return record.RecordFromArtwork(item) },
		Print: func(items []record.Artwork, start int) error {
			return display.PrintArtworksRanked(a.env.Out, a.env.Language, items, start)
		},
	}, fetch)
}
