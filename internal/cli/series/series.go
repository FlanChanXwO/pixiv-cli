// Package series owns the Pixiv artwork and novel series listing command.
package series

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	pixivcmd "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivcmd"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/resolver"
	record "github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

type options struct {
	command.Options
	ndjson bool
	limit  int
	page   int
	typ    string
}

type commandRunner struct {
	env    command.Env
	runner pixivcmd.Runner
	exec   pixivcmd.Executor
}

// New builds the actual `pixiv series` command.
func New(env command.Env, exec pixivcmd.Executor) *cobra.Command {
	a := commandRunner{env: env, runner: pixivcmd.New(env.Out, exec), exec: exec}
	options := options{}
	cmd := &cobra.Command{
		Use:   "series SERIES_ID_OR_URL",
		Short: "List the artworks or novels in a series",
		Args:  env.ExactArgs(1, "pixiv series [options] SERIES_ID_OR_URL"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.run(cmd, args[0], options)
		},
	}
	env.BindCommonFlags(cmd, &options.Options)
	pixivcmd.BindNDJSONFlag(cmd, &options.ndjson)
	pixivcmd.BindListFlags(cmd, &options.limit, &options.page)
	cmd.Flags().StringVarP(&options.typ, "type", "t", "", "entity type: artwork or novel (required)")
	env.BindTextValue(cmd, 1, 1, 0)
	command.Bind(cmd, command.PixivData())
	return cmd
}

func (a commandRunner) run(cmd *cobra.Command, arg string, opts options) error {
	if !cmd.Flags().Changed("type") {
		return errors.New("--type is required for series")
	}
	if err := validateSeriesType(opts.typ); err != nil {
		return err
	}
	target, err := resolver.Resolve(cmd.Context(), resolver.Input{Value: arg, Type: opts.typ}, seriesContract())
	if err != nil {
		return err
	}
	plan, err := command.ParsePlan(cmd, opts.limit, opts.page)
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
	ndjson := a.env.AutoNDJSON(cmd, opts.ndjson, jsonOut)

	switch target.ResultKind {
	case resolver.ResultKindArtwork:
		fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
			result, err := client.ArtworkSeries(ctx, pixiv.ArtworkSeriesRequest{SeriesID: target.ID, Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			return result.Items, result.Next, nil
		}
		return a.runner.RunArtworks(cmd.Context(), pixivcmd.Request(request), plan, pixivcmd.FormatOf(jsonOut, ndjson), pixivcmd.ArtworkList{Heading: func() string { return fmt.Sprintf("artworks in series %d", target.ID) }, Fetch: fetch, Print: func(items []pixiv.Artwork, start int) error {
			return pixivcmd.PrintArtworksRanked(a.env.Out, items, start)
		}})
	case resolver.ResultKindNovel:
		return a.runNovel(cmd, request, target.ID, plan, jsonOut, ndjson)
	default:
		return errors.New("type must be one of artwork, novel")
	}
}

func validateSeriesType(value string) error {
	switch value {
	case "artwork", "novel":
		return nil
	default:
		return errors.New("type must be one of artwork, novel")
	}
}

func seriesContract() resolver.Contract {
	return resolver.Contract{
		Operation: "series",
		Types: []resolver.TypeSpec{
			{Name: "artwork", ResultKind: resolver.ResultKindArtwork, BareReferenceKind: pixiv.ReferenceKindArtworkSeries},
			{Name: "novel", ResultKind: resolver.ResultKindNovel, BareReferenceKind: pixiv.ReferenceKindNovelSeries},
		},
		URLKinds: map[pixiv.ReferenceKind]resolver.URLTypeRelation{
			pixiv.ReferenceKindArtworkSeries: resolver.URLTypeMustMatchResult,
			pixiv.ReferenceKindNovelSeries:   resolver.URLTypeMustMatchResult,
		},
	}
}

type novelSeriesOut struct {
	Series pixiv.NovelSeries `json:"series"`
	Novels []pixiv.Novel     `json:"novels"`
}

func (a commandRunner) runNovel(cmd *cobra.Command, request pixivcmd.Request, id int64, plan command.Plan, jsonOut, ndjson bool) error {
	return a.exec(cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) (bool, error) {
		var metadata pixiv.NovelSeries
		var novels []pixiv.Novel
		fetch := func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
			result, err := client.NovelSeries(ctx, pixiv.NovelSeriesRequest{SeriesID: id, Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			if metadata.ID == 0 {
				metadata = result.Series
			}
			return result.Novels.Items, result.Novels.Next, nil
		}
		if err := command.PageItems(ctx, plan, fetch, func(items []pixiv.Novel) error {
			novels = append(novels, items...)
			return nil
		}); err != nil {
			return false, err
		}
		if ndjson {
			encoder := json.NewEncoder(a.env.Out)
			for _, novel := range novels {
				record, err := record.RecordFromNovel(novel)
				if err != nil {
					return false, err
				}
				if err := encoder.Encode(record); err != nil {
					return true, err
				}
			}
			return len(novels) > 0, nil
		}
		if jsonOut {
			if err := a.env.PrintJSON(novelSeriesOut{Series: metadata, Novels: novels}); err != nil {
				return true, err
			}
			return true, nil
		}
		if _, err := fmt.Fprintf(a.env.Out, "series %d: %s\n", metadata.ID, metadata.Title); err != nil {
			return true, err
		}
		if err := pixivcmd.PrintNovels(a.env.Out, novels); err != nil {
			return true, err
		}
		return true, nil
	})
}
