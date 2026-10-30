// Package timeline 提供 pixiv timeline 命令组：关注用户的新作品与全站最新作品、小说。
package timeline

import (
	"context"
	"errors"
	"fmt"
	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"

	"github.com/FlanChanXwO/pixiv-cli/internal/display"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/searchfilter"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/target"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

// options 保存关注新作与全站最新命令的共同选项。内容类型由边缘层显式选择，
// 这样不会把插画、漫画和小说的独立 App 分页流错误合并。
type options struct {
	environment.Options
	ndjson      bool
	limit       int
	page        int
	contentType string
	artworkType string
	restrict    string
}

type commandRunner struct {
	env environment.Env
	run session.Runner
}

// New 构造 `pixiv timeline` 命令组；following 与 latest 共用分页、代理覆盖和输出判定。
func New(env environment.Env, exec session.Executor) *cobra.Command {
	a := commandRunner{env: env, run: session.NewRunner(env.Out, exec)}
	cmd := &cobra.Command{Use: "timeline", Short: "Browse authenticated Pixiv timelines"}
	cmd.AddCommand(a.newFollowingCommand(), a.newLatestCommand())
	env.BindNoInput(cmd)
	environment.Bind(cmd, environment.Normal())
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
	environment.BindNDJSONFlag(cmd, &opts.ndjson)
	environment.BindListFlags(cmd, &opts.limit, &opts.page)
	cmd.Flags().StringVarP(&opts.contentType, "type", "t", "", "required entity type: artwork or novel")
	cmd.Flags().StringVar(&opts.artworkType, "content-type", string(pixiv.SearchContentTypeAll), "artwork subtype: all, illust-and-ugoira, illust, manga, ugoira")
	cmd.Flags().StringVar(&opts.restrict, "restrict", opts.restrict, "follow visibility: public or private")
	a.env.BindNoInput(cmd)
	environment.Bind(cmd, environment.Normal())
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
	environment.BindNDJSONFlag(cmd, &opts.ndjson)
	environment.BindListFlags(cmd, &opts.limit, &opts.page)
	cmd.Flags().StringVarP(&opts.contentType, "type", "t", "", "required entity type: artwork or novel; use --content-type for subtype")
	cmd.Flags().StringVar(&opts.artworkType, "content-type", opts.artworkType, "artwork subtype: illust or manga")
	a.env.BindNoInput(cmd)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a commandRunner) runFollowing(cmd *cobra.Command, opts options) error {
	switch pixiv.Restrict(opts.restrict) {
	case "", pixiv.RestrictPublic, pixiv.RestrictPrivate:
	default:
		return errors.New("restrict must be one of: public, private; see pixiv timeline following --help")
	}

	entity := opts.contentType
	if entity == string(target.SubtypeIllust) {
		entity = string(target.ResultKindArtwork)
	}
	if entity != string(target.ResultKindArtwork) && entity != string(target.ResultKindNovel) {
		return errors.New("type must be one of: artwork, novel")
	}
	var artworkFilter searchfilter.Filter
	if entity == string(target.ResultKindArtwork) {
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
	if entity == string(target.ResultKindArtwork) {
		fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
			result, err := client.FollowingArtworks(ctx, pixiv.FollowingArtworksRequest{Restrict: pixiv.Restrict(opts.restrict), Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			// following upstream 只提供 restrict，没有 content_type；必须在保留
			// upstream cursor 的同时本地筛选，才能让 logical limit 跨批次补足。
			return filterFollowingArtworks(result.Items, artworkFilter), result.Next, nil
		}
		return record.RunArtworks(cmd.Context(), a.run, request, plan, record.FormatOf(jsonOut, ndjson), record.ArtworkList{Heading: record.StaticHeading("new artworks from followed users"), Fetch: fetch, Print: func(items []pixiv.Artwork, _ int) error { return display.PrintArtworks(a.env.Out, items) }})
	}
	fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
		result, err := client.FollowingNovels(ctx, pixiv.FollowingNovelsRequest{Restrict: pixiv.Restrict(opts.restrict), Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return record.RunNovels(cmd.Context(), a.run, request, plan, record.FormatOf(jsonOut, ndjson), record.NovelList{Heading: "new novels from followed users", Fetch: fetch, Print: func(items []pixiv.Novel) error { return display.PrintNovels(a.env.Out, items) }})
}

func (a commandRunner) runLatest(cmd *cobra.Command, opts options) error {
	entity := opts.contentType
	if entity == string(target.SubtypeIllust) || entity == string(target.SubtypeManga) {
		if cmd.Flags().Changed("content-type") {
			return errors.New("legacy artwork --type cannot be combined with --content-type")
		}
		opts.artworkType = entity
		entity = string(target.ResultKindArtwork)
	}
	if entity != string(target.ResultKindArtwork) && entity != string(target.ResultKindNovel) {
		return errors.New("type must be one of: artwork, novel")
	}
	if entity == string(target.ResultKindNovel) && cmd.Flags().Changed("content-type") {
		return errors.New("--content-type is only supported when --type artwork")
	}
	if entity == string(target.ResultKindArtwork) && opts.artworkType != string(pixiv.SearchContentTypeIllust) && opts.artworkType != string(pixiv.SearchContentTypeManga) {
		return errors.New("content-type must be one of: illust, manga")
	}
	plan, request, jsonOut, ndjson, err := a.resolve(cmd, opts)
	if err != nil {
		return err
	}
	if entity == string(target.ResultKindNovel) {
		fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
			result, err := client.LatestNovels(ctx, pixiv.LatestNovelsRequest{Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			return result.Items, result.Next, nil
		}
		return record.RunNovels(cmd.Context(), a.run, request, plan, record.FormatOf(jsonOut, ndjson), record.NovelList{Heading: "latest novels", Fetch: fetch, Print: func(items []pixiv.Novel) error { return display.PrintNovels(a.env.Out, items) }})
	}
	fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		result, err := client.LatestArtworks(ctx, pixiv.LatestArtworksRequest{ContentType: pixiv.SearchContentType(opts.artworkType), Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return record.RunArtworks(cmd.Context(), a.run, request, plan, record.FormatOf(jsonOut, ndjson), record.ArtworkList{Heading: record.StaticHeading(fmt.Sprintf("latest %s", opts.artworkType)), Fetch: fetch, Print: func(items []pixiv.Artwork, _ int) error { return display.PrintArtworks(a.env.Out, items) }})
}

// resolve 统一两个 timeline 命令的分页、传输覆盖与输出模式判定，使 --ndjson 与
// --json 的互斥、自动 NDJSON 判定和 JSON 解析顺序保持一致。
func (a commandRunner) resolve(cmd *cobra.Command, opts options) (pagination.PagePlan, session.ProxyRequest, bool, bool, error) {
	plan, err := environment.ParsePlan(cmd, opts.limit, opts.page)
	if err != nil {
		return pagination.PagePlan{}, session.ProxyRequest{}, false, false, err
	}
	request, err := a.env.Request(cmd, opts.Options.ProxyOptions)
	if err != nil {
		return pagination.PagePlan{}, session.ProxyRequest{}, false, false, err
	}
	if opts.ndjson && cmd.Flags().Changed("json") {
		return pagination.PagePlan{}, session.ProxyRequest{}, false, false, a.env.WrapUsage(errors.New("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !opts.ndjson {
		jsonOut, err = a.env.ResolveJSON(a.env.JSONOverride(cmd, opts.Options))
		if err != nil {
			return pagination.PagePlan{}, session.ProxyRequest{}, false, false, err
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
