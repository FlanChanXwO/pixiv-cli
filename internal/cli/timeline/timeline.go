// Package timeline owns the authenticated `pixiv timeline` command group.
package timeline

import (
	"context"
	"errors"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	pixivcmd "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivcmd"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/resolver"
	"github.com/FlanChanXwO/pixiv-cli/internal/searchfilter"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

// options 是关注新作与全站最新命令的共同选项。内容类型由边缘层显式选择，
// 这样不会把插画、漫画和小说的独立 App 分页流错误合并。
type options struct {
	command.Options
	ndjson      bool
	limit       int
	page        int
	contentType string
	artworkType string
	restrict    string
}

type commandRunner struct {
	env command.Env
	run pixivcmd.Runner
}

// New builds the actual `pixiv timeline` command group.
func New(env command.Env, exec pixivcmd.Executor) *cobra.Command {
	a := commandRunner{env: env, run: pixivcmd.New(env.Out, exec)}
	cmd := &cobra.Command{Use: "timeline", Short: "Browse authenticated Pixiv timelines"}
	cmd.AddCommand(a.newFollowingCommand(), a.newLatestCommand())
	env.BindNoInput(cmd)
	command.Bind(cmd, command.PixivData())
	return cmd
}

func (a commandRunner) newFollowingCommand() *cobra.Command {
	opts := options{restrict: string(pixiv.RestrictPublic)}
	cmd := &cobra.Command{
		Use:   "following",
		Short: "Browse new works from followed users",
		Args:  a.env.ExactArgs(0, "pixiv timeline following --type artwork|novel"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.runFollowing(cmd, opts)
		},
	}
	a.env.BindCommonFlags(cmd, &opts.Options)
	pixivcmd.BindNDJSONFlag(cmd, &opts.ndjson)
	pixivcmd.BindListFlags(cmd, &opts.limit, &opts.page)
	cmd.Flags().StringVarP(&opts.contentType, "type", "t", "", "required entity type: artwork or novel")
	cmd.Flags().StringVar(&opts.artworkType, "content-type", string(pixiv.SearchContentTypeAll), "artwork subtype: all, illust-and-ugoira, illust, manga, ugoira")
	cmd.Flags().StringVar(&opts.restrict, "restrict", opts.restrict, "follow visibility: public or private")
	a.env.BindNoInput(cmd)
	command.Bind(cmd, command.PixivData())
	return cmd
}

func (a commandRunner) newLatestCommand() *cobra.Command {
	opts := options{artworkType: string(pixiv.SearchContentTypeIllust)}
	cmd := &cobra.Command{
		Use:   "latest",
		Short: "Browse Pixiv's latest works",
		Args:  a.env.ExactArgs(0, "pixiv timeline latest --type artwork|novel [--content-type illust|manga]"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.runLatest(cmd, opts)
		},
	}
	a.env.BindCommonFlags(cmd, &opts.Options)
	pixivcmd.BindNDJSONFlag(cmd, &opts.ndjson)
	pixivcmd.BindListFlags(cmd, &opts.limit, &opts.page)
	cmd.Flags().StringVarP(&opts.contentType, "type", "t", "", "required entity type: artwork or novel; use --content-type for subtype")
	cmd.Flags().StringVar(&opts.artworkType, "content-type", opts.artworkType, "artwork subtype: illust or manga")
	a.env.BindNoInput(cmd)
	command.Bind(cmd, command.PixivData())
	return cmd
}

func (a commandRunner) runFollowing(cmd *cobra.Command, opts options) error {
	entity := opts.contentType
	if entity == string(resolver.SubtypeIllust) {
		entity = string(resolver.ResultKindArtwork)
	}
	if entity != string(resolver.ResultKindArtwork) && entity != string(resolver.ResultKindNovel) {
		return errors.New("type must be one of: artwork, novel")
	}
	var artworkFilter searchfilter.Filter
	if entity == string(resolver.ResultKindArtwork) {
		contentType, err := searchfilter.NormalizeContentType(opts.artworkType)
		if err != nil {
			return err
		}
		artworkFilter.ContentType = contentType
	} else if cmd.Flags().Changed("content-type") {
		return errors.New("--content-type is only supported when --type artwork")
	}
	plan, request, jsonOut, ndjson, err := a.resolve(cmd, opts)
	if err != nil {
		return err
	}
	if entity == string(resolver.ResultKindArtwork) {
		fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
			result, err := client.FollowingArtworks(ctx, pixiv.FollowingArtworksRequest{Restrict: pixiv.Restrict(opts.restrict), Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			// following upstream 只提供 restrict，没有 content_type；必须在保留
			// upstream cursor 的同时本地筛选，才能让 logical limit 跨批次补足。
			return filterFollowingArtworks(result.Items, artworkFilter), result.Next, nil
		}
		return a.run.RunArtworks(cmd.Context(), pixivcmd.Request(request), plan, pixivcmd.FormatOf(jsonOut, ndjson), pixivcmd.ArtworkList{Heading: pixivcmd.Static("new artworks from followed users"), Fetch: fetch, Print: func(items []pixiv.Artwork, _ int) error { return pixivcmd.PrintArtworks(a.env.Out, items) }})
	}
	fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
		result, err := client.FollowingNovels(ctx, pixiv.FollowingNovelsRequest{Restrict: pixiv.Restrict(opts.restrict), Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return a.run.RunNovels(cmd.Context(), pixivcmd.Request(request), plan, pixivcmd.FormatOf(jsonOut, ndjson), pixivcmd.NovelList{Heading: "new novels from followed users", Fetch: fetch, Print: func(items []pixiv.Novel) error { return pixivcmd.PrintNovels(a.env.Out, items) }})
}

func (a commandRunner) runLatest(cmd *cobra.Command, opts options) error {
	entity := opts.contentType
	if entity == string(resolver.SubtypeIllust) || entity == string(resolver.SubtypeManga) {
		if cmd.Flags().Changed("content-type") {
			return errors.New("legacy artwork --type cannot be combined with --content-type")
		}
		opts.artworkType = entity
		entity = string(resolver.ResultKindArtwork)
	}
	if entity != string(resolver.ResultKindArtwork) && entity != string(resolver.ResultKindNovel) {
		return errors.New("type must be one of: artwork, novel")
	}
	if entity == string(resolver.ResultKindNovel) && cmd.Flags().Changed("content-type") {
		return errors.New("--content-type is only supported when --type artwork")
	}
	if entity == string(resolver.ResultKindArtwork) && opts.artworkType != string(pixiv.SearchContentTypeIllust) && opts.artworkType != string(pixiv.SearchContentTypeManga) {
		return errors.New("content-type must be one of: illust, manga")
	}
	plan, request, jsonOut, ndjson, err := a.resolve(cmd, opts)
	if err != nil {
		return err
	}
	if entity == string(resolver.ResultKindNovel) {
		fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
			result, err := client.LatestNovels(ctx, pixiv.LatestNovelsRequest{Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			return result.Items, result.Next, nil
		}
		return a.run.RunNovels(cmd.Context(), pixivcmd.Request(request), plan, pixivcmd.FormatOf(jsonOut, ndjson), pixivcmd.NovelList{Heading: "latest novels", Fetch: fetch, Print: func(items []pixiv.Novel) error { return pixivcmd.PrintNovels(a.env.Out, items) }})
	}
	fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		result, err := client.LatestArtworks(ctx, pixiv.LatestArtworksRequest{ContentType: pixiv.SearchContentType(opts.artworkType), Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return a.run.RunArtworks(cmd.Context(), pixivcmd.Request(request), plan, pixivcmd.FormatOf(jsonOut, ndjson), pixivcmd.ArtworkList{Heading: pixivcmd.Static(fmt.Sprintf("latest %s", opts.artworkType)), Fetch: fetch, Print: func(items []pixiv.Artwork, _ int) error { return pixivcmd.PrintArtworks(a.env.Out, items) }})
}

// resolve 统一两个 timeline 命令的分页、传输覆盖与输出模式判定，使 --ndjson 与
// --json 的互斥、自动 NDJSON 判定和 JSON 解析顺序保持一致。
func (a commandRunner) resolve(cmd *cobra.Command, opts options) (command.Plan, pixivcmd.Request, bool, bool, error) {
	plan, err := command.ParsePlan(cmd, opts.limit, opts.page)
	if err != nil {
		return command.Plan{}, pixivcmd.Request{}, false, false, err
	}
	request, err := pixivcmd.BuildRequest(cmd, opts.Options)
	if err != nil {
		return command.Plan{}, pixivcmd.Request{}, false, false, err
	}
	if opts.ndjson && cmd.Flags().Changed("json") {
		return command.Plan{}, pixivcmd.Request{}, false, false, a.env.UsageError(errors.New("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !opts.ndjson {
		jsonOut, err = a.env.JSONOut(a.env.JSONOverride(cmd, opts.Options))
		if err != nil {
			return command.Plan{}, pixivcmd.Request{}, false, false, err
		}
	}
	return plan, request, jsonOut, a.env.AutoNDJSON(cmd, opts.ndjson, jsonOut), nil
}

func filterFollowingArtworks(items []pixiv.Artwork, filter searchfilter.Filter) []pixiv.Artwork {
	if filter.ContentType == "" || filter.ContentType == searchfilter.ContentTypeAll {
		return items
	}
	filtered := make([]pixiv.Artwork, 0, len(items))
	for _, item := range items {
		if filter.Matches(item.XRestrict, string(item.Kind)) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}
