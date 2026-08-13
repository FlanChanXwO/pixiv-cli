// Package ranking owns the Pixiv illustration ranking command.
package ranking

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivdeps"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pixiv/internal/listing"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/requirements"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

type options struct {
	deps.CommandOptions
	ndjson bool
	mode   string
	date   string
	limit  int
	page   int
}

type command struct {
	data deps.Data
}

// New builds the actual `pixiv ranking` command.
func New(data deps.Data) *cobra.Command {
	a := command{data: data}
	options := options{mode: string(pixiv.RankingModeDay)}
	cmd := &cobra.Command{
		Use:   "ranking",
		Short: "Show illustration ranking",
		Args:  data.ExactArgs(0, "pixiv ranking [options]"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.run(cmd, options)
		},
	}
	data.BindCommonFlags(cmd, &options.CommandOptions)
	listing.BindNDJSONFlag(cmd, &options.ndjson)
	flags := cmd.Flags()
	flags.StringVar(&options.mode, "mode", options.mode, "ranking mode: day, day_male, day_female, week, week_original, week_rookie, month, day_manga, week_manga, month_manga, week_rookie_manga, day_r18, day_male_r18, day_female_r18, week_r18, week_r18g; the last nine require authentication")
	flags.StringVar(&options.date, "date", "", "YYYY-MM-DD")
	listing.BindListFlags(cmd, &options.limit, &options.page)
	data.BindNoInput(cmd)
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

func (a command) run(cmd *cobra.Command, opts options) error {
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
	fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		result, err := client.ArtworkRanking(ctx, pixiv.ArtworkRankingRequest{Mode: pixiv.RankingMode(opts.mode), Date: opts.date, Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return listing.New(a.data.Output, a.data).RunPooledIllustList(cmd.Context(), request, plan, jsonOut, ndjson, fmt.Sprintf("%s ranking", opts.mode), fetch, func(items []pixiv.Artwork, start int) error {
		return printArtworks(a.data.Output, items, start)
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
