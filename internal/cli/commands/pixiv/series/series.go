// Package series owns the Pixiv artwork and novel series listing command.
package series

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	requirements "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/internal/listing"
	record "github.com/FlanChanXwO/pixiv-cli/internal/shared/record"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/parse"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

type options struct {
	deps.CommandOptions
	ndjson bool
	limit  int
	page   int
	typ    string
}

type command struct {
	data deps.Data
}

// New builds the actual `pixiv series` command.
func New(data deps.Data) *cobra.Command {
	a := command{data: data}
	options := options{}
	cmd := &cobra.Command{
		Use:   "series SERIES_ID",
		Short: "List the artworks or novels in a series",
		Args:  data.ExactArgs(1, "pixiv series [options] SERIES_ID"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.run(cmd, args[0], options)
		},
	}
	data.BindCommonFlags(cmd, &options.CommandOptions)
	listing.BindNDJSONFlag(cmd, &options.ndjson)
	listing.BindListFlags(cmd, &options.limit, &options.page)
	cmd.Flags().StringVarP(&options.typ, "type", "t", "", "entity type: artwork or novel (required)")
	data.BindTextValue(cmd, 1, 1, 0)
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

func (a command) run(cmd *cobra.Command, arg string, opts options) error {
	if !cmd.Flags().Changed("type") {
		return errors.New("--type is required for series")
	}
	id, err := parse.PositiveInt64(arg, "series_id")
	if err != nil {
		return err
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
	ndjson := a.data.ShouldAutoNDJSON(cmd, opts.ndjson, jsonOut)

	switch opts.typ {
	case "artwork":
		fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
			result, err := client.ArtworkSeries(ctx, pixiv.ArtworkSeriesRequest{SeriesID: id, Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			return result.Items, result.Next, nil
		}
		return a.runner().RunPooledIllustListWithHeading(cmd.Context(), listing.Request(request), plan, jsonOut, ndjson,
			func() string { return fmt.Sprintf("artworks in series %d", id) }, fetch,
			func(items []pixiv.Artwork, start int) error { return printArtworks(a.data.Output, items, start) })
	case "novel":
		return a.runNovel(cmd, request, id, plan, jsonOut, ndjson)
	default:
		return errors.New("type must be one of artwork, novel")
	}
}

// runner is resolved only after Cobra has accepted the command input and the
// root has prepared the command's declared resources.
func (a command) runner() listing.Runner {
	return listing.New(a.data.Output, func(ctx context.Context, request listing.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
		return a.data.Pooled(ctx, deps.Request(request), attempt)
	})
}

type novelSeriesOut struct {
	Series pixiv.NovelSeriesDTO `json:"series"`
	Novels []pixiv.NovelDTO     `json:"novels"`
}

func (a command) runNovel(cmd *cobra.Command, request deps.Request, id int64, plan listing.Plan, jsonOut, ndjson bool) error {
	return a.data.Pooled(cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) (bool, error) {
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
		if err := listing.PageItems(ctx, plan, fetch, func(items []pixiv.Novel) error {
			novels = append(novels, items...)
			return nil
		}); err != nil {
			return false, err
		}
		if ndjson {
			encoder := json.NewEncoder(a.data.Output)
			for _, novel := range novels {
				record, err := record.RecordFromNovelDTO(pixiv.ToNovelDTO(novel))
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
			novelDTOs := make([]pixiv.NovelDTO, 0, len(novels))
			for _, novel := range novels {
				novelDTOs = append(novelDTOs, pixiv.ToNovelDTO(novel))
			}
			if err := a.data.WriteJSON(novelSeriesOut{Series: pixiv.ToNovelSeriesDTO(metadata), Novels: novelDTOs}); err != nil {
				return true, err
			}
			return true, nil
		}
		if _, err := fmt.Fprintf(a.data.Output, "series %d: %s\n", metadata.ID, metadata.Title); err != nil {
			return true, err
		}
		if err := printNovels(a.data.Output, novels); err != nil {
			return true, err
		}
		return true, nil
	})
}

func printArtworks(out io.Writer, items []pixiv.Artwork, offset int) error {
	for index, item := range items {
		if _, err := fmt.Fprintf(out, "#%d https://www.pixiv.net/artworks/%d\n", offset+index+1, item.ID); err != nil {
			return err
		}
		tags := make([]string, 0, len(item.Tags))
		for _, tag := range item.Tags {
			tags = append(tags, tag.Name)
		}
		if _, err := fmt.Fprintf(out, "%d %q by %s bookmarks:%d views:%d tags:%s\n", item.ID, item.Title, item.User.Name, item.TotalBookmarks, item.TotalViews, strings.Join(tags, ",")); err != nil {
			return err
		}
	}
	return nil
}

func printNovels(out io.Writer, items []pixiv.Novel) error {
	for _, item := range items {
		if _, err := fmt.Fprintf(out, "%d %s — %s\n", item.ID, item.Title, item.User.Name); err != nil {
			return err
		}
	}
	return nil
}
