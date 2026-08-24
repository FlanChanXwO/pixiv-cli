// Package timeline owns the authenticated `pixiv timeline` command group.
package timeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	requirements "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands"
	deps "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/internal/listing"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

// options 是关注新作与全站最新命令的共同选项。内容类型由边缘层显式选择，
// 这样不会把插画、漫画和小说的独立 App 分页流错误合并。
type options struct {
	deps.CommandOptions
	ndjson      bool
	limit       int
	page        int
	contentType string
	artworkType string
	restrict    string
}

type command struct {
	data deps.Data
}

// New builds the actual `pixiv timeline` command group.
func New(data deps.Data) *cobra.Command {
	a := command{data: data}
	cmd := &cobra.Command{Use: "timeline", Short: "Browse authenticated Pixiv timelines"}
	cmd.AddCommand(a.newFollowingCommand(), a.newLatestCommand())
	data.BindNoInput(cmd)
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

func (a command) newFollowingCommand() *cobra.Command {
	opts := options{restrict: string(pixiv.RestrictPublic)}
	cmd := &cobra.Command{
		Use:   "following",
		Short: "Browse new works from followed users",
		Args:  a.data.ExactArgs(0, "pixiv timeline following --type illust|novel"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.runFollowing(cmd, opts)
		},
	}
	a.data.BindCommonFlags(cmd, &opts.CommandOptions)
	listing.BindNDJSONFlag(cmd, &opts.ndjson)
	listing.BindListFlags(cmd, &opts.limit, &opts.page)
	cmd.Flags().StringVarP(&opts.contentType, "type", "t", "", "required entity type: artwork or novel")
	cmd.Flags().StringVar(&opts.artworkType, "content-type", string(pixiv.SearchContentTypeAll), "artwork subtype: all, illust-and-ugoira, illust, manga, ugoira")
	cmd.Flags().StringVar(&opts.restrict, "restrict", opts.restrict, "follow visibility: public or private")
	a.data.BindNoInput(cmd)
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

func (a command) newLatestCommand() *cobra.Command {
	opts := options{artworkType: string(pixiv.SearchContentTypeIllust)}
	cmd := &cobra.Command{
		Use:   "latest",
		Short: "Browse Pixiv's latest works",
		Args:  a.data.ExactArgs(0, "pixiv timeline latest --type artwork|novel [--content-type illust|manga]"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.runLatest(cmd, opts)
		},
	}
	a.data.BindCommonFlags(cmd, &opts.CommandOptions)
	listing.BindNDJSONFlag(cmd, &opts.ndjson)
	listing.BindListFlags(cmd, &opts.limit, &opts.page)
	cmd.Flags().StringVarP(&opts.contentType, "type", "t", "", "required entity type: artwork or novel; use --content-type for subtype")
	cmd.Flags().StringVar(&opts.artworkType, "content-type", opts.artworkType, "artwork subtype: illust or manga")
	a.data.BindNoInput(cmd)
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

func (a command) runFollowing(cmd *cobra.Command, opts options) error {
	entity := opts.contentType
	if entity == "illust" {
		entity = "artwork"
	}
	if entity != "artwork" && entity != "novel" {
		return errors.New("type must be one of: artwork, novel")
	}
	plan, request, jsonOut, ndjson, err := a.resolve(cmd, opts)
	if err != nil {
		return err
	}
	if entity == "artwork" {
		fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
			result, err := client.FollowingArtworks(ctx, pixiv.FollowingArtworksRequest{Restrict: pixiv.Restrict(opts.restrict), Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			return result.Items, result.Next, nil
		}
		return a.runner().RunPooledIllustList(cmd.Context(), listing.Request(request), plan, jsonOut, ndjson, "new artworks from followed users", fetch,
			func(items []pixiv.Artwork, start int) error { return printArtworks(a.data.Output, items) })
	}
	fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
		result, err := client.FollowingNovels(ctx, pixiv.FollowingNovelsRequest{Restrict: pixiv.Restrict(opts.restrict), Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return a.runner().RunPooledNovelList(cmd.Context(), listing.Request(request), plan, jsonOut, ndjson, "new novels from followed users", fetch,
		func(items []pixiv.Novel) error { return printNovels(a.data.Output, items) })
}

func (a command) runLatest(cmd *cobra.Command, opts options) error {
	entity := opts.contentType
	if entity == "illust" || entity == "manga" {
		if cmd.Flags().Changed("content-type") {
			return errors.New("legacy artwork --type cannot be combined with --content-type")
		}
		opts.artworkType = entity
		entity = "artwork"
	}
	if entity != "artwork" && entity != "novel" {
		return errors.New("type must be one of: artwork, novel")
	}
	if entity == "artwork" && opts.artworkType != string(pixiv.SearchContentTypeIllust) && opts.artworkType != string(pixiv.SearchContentTypeManga) {
		return errors.New("content-type must be one of: illust, manga")
	}
	plan, request, jsonOut, ndjson, err := a.resolve(cmd, opts)
	if err != nil {
		return err
	}
	if entity == "novel" {
		fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
			result, err := client.LatestNovels(ctx, pixiv.LatestNovelsRequest{Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			return result.Items, result.Next, nil
		}
		return a.runner().RunPooledNovelList(cmd.Context(), listing.Request(request), plan, jsonOut, ndjson, "latest novels", fetch,
			func(items []pixiv.Novel) error { return printNovels(a.data.Output, items) })
	}
	fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		result, err := client.LatestArtworks(ctx, pixiv.LatestArtworksRequest{ContentType: pixiv.SearchContentType(opts.artworkType), Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return a.runner().RunPooledIllustList(cmd.Context(), listing.Request(request), plan, jsonOut, ndjson, fmt.Sprintf("latest %s", opts.artworkType), fetch,
		func(items []pixiv.Artwork, start int) error { return printArtworks(a.data.Output, items) })
}

// resolve 统一两个 timeline 命令的分页、传输覆盖与输出模式判定，使 --ndjson 与
// --json 的互斥、自动 NDJSON 判定和 JSON 解析顺序保持一致。
func (a command) resolve(cmd *cobra.Command, opts options) (listing.Plan, deps.Request, bool, bool, error) {
	plan, err := listing.ParsePlan(cmd, opts.limit, opts.page)
	if err != nil {
		return listing.Plan{}, deps.Request{}, false, false, err
	}
	request, err := a.data.Request(cmd, opts.CommandOptions)
	if err != nil {
		return listing.Plan{}, deps.Request{}, false, false, err
	}
	if opts.ndjson && cmd.Flags().Changed("json") {
		return listing.Plan{}, deps.Request{}, false, false, a.data.Usage(errors.New("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !opts.ndjson {
		jsonOut, err = a.data.JSONOut(a.data.JSONOverride(cmd, opts.CommandOptions))
		if err != nil {
			return listing.Plan{}, deps.Request{}, false, false, err
		}
	}
	return plan, request, jsonOut, a.data.ShouldAutoNDJSON(cmd, opts.ndjson, jsonOut), nil
}

// runner is resolved only after Cobra has accepted the command input and the
// root has prepared the command's declared resources.
func (a command) runner() listing.Runner {
	return listing.New(a.data.Output, func(ctx context.Context, request listing.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
		return a.data.Pooled(ctx, deps.Request(request), attempt)
	})
}

func printArtworks(out io.Writer, items []pixiv.Artwork) error {
	for _, item := range items {
		if _, err := fmt.Fprintf(out, "https://www.pixiv.net/artworks/%d\n", item.ID); err != nil {
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
