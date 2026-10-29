// Package series 提供 pixiv series 命令，列出作品或小说系列的条目。
package series

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/display"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
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
	limit  int
	page   int
	typ    string
}

type commandRunner struct {
	env    environment.Env
	runner session.Runner
	exec   session.Executor
}

// New 构造 `pixiv series` 命令；SERIES_ID_OR_URL 由请求解析器判定作品或小说系列。
func New(env environment.Env, exec session.Executor) *cobra.Command {
	a := commandRunner{env: env, runner: session.NewRunner(env.Out, exec), exec: exec}
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
	environment.BindNDJSONFlag(cmd, &options.ndjson)
	environment.BindListFlags(cmd, &options.limit, &options.page)
	cmd.Flags().StringVarP(&options.typ, "type", "t", "", "entity type: artwork or novel (required)")
	env.BindTextValue(cmd, 1, 1, 0)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a commandRunner) run(cmd *cobra.Command, arg string, opts options) error {
	if !cmd.Flags().Changed("type") {
		return errors.New("--type is required for series")
	}
	if err := validateSeriesType(opts.typ); err != nil {
		return err
	}
	resolved, err := target.Resolve(cmd.Context(), target.Input{Value: arg, Type: opts.typ}, seriesContract())
	if err != nil {
		return err
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
	ndjson := a.env.AutoNDJSON(cmd, opts.ndjson, jsonOut)

	switch resolved.ResultKind {
	case target.ResultKindArtwork:
		fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
			result, err := client.ArtworkSeries(ctx, pixiv.ArtworkSeriesRequest{SeriesID: resolved.ID, Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			return result.Items, result.Next, nil
		}
		return record.RunArtworks(cmd.Context(), a.runner, request, plan, record.FormatOf(jsonOut, ndjson), record.ArtworkList{Heading: func() string { return fmt.Sprintf("artworks in series %d", resolved.ID) }, Fetch: fetch, Print: func(items []pixiv.Artwork, start int) error {
			return display.PrintArtworksRanked(a.env.Out, items, start)
		}})
	case target.ResultKindNovel:
		return a.runNovel(cmd, request, resolved.ID, plan, jsonOut, ndjson)
	default:
		return errors.New("type must be one of artwork, novel")
	}
}

func validateSeriesType(value string) error {
	switch value {
	case string(target.ResultKindArtwork), string(target.ResultKindNovel):
		return nil
	default:
		return errors.New("type must be one of artwork, novel")
	}
}

func seriesContract() target.Contract {
	return target.Contract{
		Operation: "series",
		Types: []target.TypeSpec{
			{Name: string(target.ResultKindArtwork), ResultKind: target.ResultKindArtwork, BareReferenceKind: pixiv.ReferenceKindArtworkSeries},
			{Name: string(target.ResultKindNovel), ResultKind: target.ResultKindNovel, BareReferenceKind: pixiv.ReferenceKindNovelSeries},
		},
		URLKinds: map[pixiv.ReferenceKind]target.URLTypeRelation{
			pixiv.ReferenceKindArtworkSeries: target.URLTypeMustMatchResult,
			pixiv.ReferenceKindNovelSeries:   target.URLTypeMustMatchResult,
		},
	}
}

type novelSeriesOut struct {
	Series pixiv.NovelSeries `json:"series"`
	Novels []pixiv.Novel     `json:"novels"`
}

func (a commandRunner) runNovel(cmd *cobra.Command, request session.ProxyRequest, id int64, plan pagination.PagePlan, jsonOut, ndjson bool) error {
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
		if err := pagination.PageItems(ctx, plan, fetch, func(items []pixiv.Novel) error {
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
		if err := display.PrintNovels(a.env.Out, novels); err != nil {
			return true, err
		}
		return true, nil
	})
}
