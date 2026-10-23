// Package ranking owns the Pixiv artwork and novel ranking command.
package ranking

import (
	"context"
	"errors"
	"fmt"

	commands "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivcmd"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

type options struct {
	commands.Options
	ndjson bool
	typ    string
	mode   string
	date   string
	limit  int
	page   int
}

type command struct {
	env    commands.Env
	runner pixivcmd.Runner
}

// New builds the actual `pixiv ranking` command.
func New(env commands.Env, exec pixivcmd.Executor) *cobra.Command {
	a := command{env: env, runner: pixivcmd.New(env.Out, exec)}
	options := options{typ: "artwork", mode: string(pixiv.RankingModeDay)}
	cmd := &cobra.Command{
		Use:   "ranking",
		Short: "Show artwork or novel ranking",
		Args:  env.ExactArgs(0, "pixiv ranking [options]"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.run(cmd, options)
		},
	}
	env.BindCommonFlags(cmd, &options.Options)
	pixivcmd.BindNDJSONFlag(cmd, &options.ndjson)
	flags := cmd.Flags()
	flags.StringVarP(&options.typ, "type", "t", options.typ, "entity type: artwork or novel")
	flags.StringVar(&options.mode, "mode", options.mode, "ranking mode: day, day_male, day_female, week, week_original, week_rookie, month, day_manga, week_manga, month_manga, week_rookie_manga, day_r18, day_male_r18, day_female_r18, week_r18, week_r18g; the last nine require authentication")
	flags.StringVar(&options.date, "date", "", "YYYY-MM-DD (artwork ranking only)")
	pixivcmd.BindListFlags(cmd, &options.limit, &options.page)
	env.BindNoInput(cmd)
	commands.Bind(cmd, commands.PixivData())
	return cmd
}

func (a command) run(cmd *cobra.Command, opts options) error {
	if opts.typ != "artwork" && opts.typ != "novel" {
		return errors.New("type must be one of: artwork, novel")
	}
	if opts.typ == "novel" && cmd.Flags().Changed("date") {
		return errors.New("--date is only supported when --type artwork")
	}
	plan, err := pixivcmd.ParsePlan(cmd, opts.limit, opts.page)
	if err != nil {
		return err
	}
	request, err := pixivcmd.BuildRequest(cmd, opts.Options)
	if err != nil {
		return err
	}
	if opts.ndjson && cmd.Flags().Changed("json") {
		return a.env.UsageError(errors.New("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !opts.ndjson {
		jsonOut, err = a.env.JSONOut(a.env.JSONOverride(cmd, opts.Options))
		if err != nil {
			return err
		}
	}
	ndjson := a.env.AutoNDJSON(cmd, opts.ndjson, jsonOut)
	format := pixivcmd.FormatOf(jsonOut, ndjson)
	if opts.typ == "novel" {
		fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
			result, err := client.NovelRanking(ctx, pixiv.NovelRankingRequest{Mode: pixiv.RankingMode(opts.mode), Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			return result.Items, result.Next, nil
		}
		return a.runner.RunNovels(cmd.Context(), request, plan, format, pixivcmd.NovelList{
			Heading: fmt.Sprintf("%s novel ranking", opts.mode),
			Fetch:   fetch,
			Print:   func(items []pixiv.Novel) error { return pixivcmd.PrintNovels(a.env.Out, items) },
		})
	}
	fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		result, err := client.ArtworkRanking(ctx, pixiv.ArtworkRankingRequest{Mode: pixiv.RankingMode(opts.mode), Date: opts.date, Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return a.runner.RunArtworks(cmd.Context(), request, plan, format, pixivcmd.ArtworkList{
		Heading: pixivcmd.Static(fmt.Sprintf("%s ranking", opts.mode)),
		Fetch:   fetch,
		Print: func(items []pixiv.Artwork, start int) error {
			return pixivcmd.PrintArtworksRanked(a.env.Out, items, start)
		},
	})
}
