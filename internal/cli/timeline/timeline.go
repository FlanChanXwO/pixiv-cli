// Package timeline 提供 pixiv timeline 命令组：关注用户的新作品与全站最新作品、小说。
package timeline

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/display"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	legacysdk "github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
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
	env  environment.Env
	exec session.SDKExecutor
}

// New 构造 `pixiv timeline` 命令组；following 与 latest 共用分页、代理覆盖和输出判定。
func New(env environment.Env, exec session.SDKExecutor) *cobra.Command {
	a := commandRunner{env: env, exec: exec}
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
	cmd.Flags().StringVar(&opts.artworkType, "content-type", string(pixiv.ArtworkCategoryAll), "artwork subtype: all, illust-and-ugoira, illust, manga, ugoira")
	cmd.Flags().StringVar(&opts.restrict, "restrict", opts.restrict, "follow visibility: public or private")
	a.env.BindNoInput(cmd)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a commandRunner) newLatestCommand() *cobra.Command {
	opts := options{artworkType: string(pixiv.ArtworkCategoryIllust)}
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
	if entity == string(pixiv.ContentTypeIllust) {
		entity = string(target.NamespaceArtwork)
	}
	if entity != string(target.NamespaceArtwork) && entity != string(target.NamespaceNovel) {
		return errors.New("type must be one of: artwork, novel")
	}
	var artworkFilter searchfilter.Filter
	if entity == string(target.NamespaceArtwork) {
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
	if entity == string(target.NamespaceArtwork) {
		fetch := func(ctx context.Context, _ int64, client *pixiv.Client, cursor sdk.Cursor) ([]legacysdk.Artwork, sdk.Cursor, error) {
			result, err := client.FollowingArtworks(ctx, pixiv.FollowingOptions{Restrict: pixiv.Restrict(opts.restrict), Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			items := make([]legacysdk.Artwork, len(result.Items))
			for i, item := range result.Items {
				items[i], err = record.ArtworkOutput(item, false)
				if err != nil {
					return nil, sdk.Cursor{}, err
				}
			}
			// 关注动态的子类型先本地筛选，逻辑 limit 才能跨批次补足。
			return filterFollowingArtworks(items, artworkFilter), result.Cursor, nil
		}
		return record.RunSDKList(cmd.Context(), a.env.Out, a.env.Err, a.exec, request, plan, record.FormatOf(jsonOut, ndjson), record.List[legacysdk.Artwork]{
			RecordValue: func(item legacysdk.Artwork) (any, error) { return record.RecordFromArtwork(item) },
			Print:       func(items []legacysdk.Artwork, _ int) error { return display.PrintArtworks(a.env.Out, items) },
		}, fetch)
	}
	fetch := func(ctx context.Context, _ int64, client *pixiv.Client, cursor sdk.Cursor) ([]legacysdk.Novel, sdk.Cursor, error) {
		result, err := client.FollowingNovels(ctx, pixiv.FollowingOptions{Restrict: pixiv.Restrict(opts.restrict), Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		items := make([]legacysdk.Novel, len(result.Items))
		for i, item := range result.Items {
			items[i], err = record.NovelOutput(item)
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
		}
		return items, result.Cursor, nil
	}
	return record.RunSDKList(cmd.Context(), a.env.Out, a.env.Err, a.exec, request, plan, record.FormatOf(jsonOut, ndjson), record.List[legacysdk.Novel]{
		RecordValue: func(item legacysdk.Novel) (any, error) { return record.RecordFromNovel(item) },
		Print:       func(items []legacysdk.Novel, _ int) error { return display.PrintNovels(a.env.Out, items) },
	}, fetch)
}

func (a commandRunner) runLatest(cmd *cobra.Command, opts options) error {
	entity := opts.contentType
	if entity == string(pixiv.ContentTypeIllust) || entity == string(pixiv.ContentTypeManga) {
		if cmd.Flags().Changed("content-type") {
			return errors.New("legacy artwork --type cannot be combined with --content-type")
		}
		opts.artworkType = entity
		entity = string(target.NamespaceArtwork)
	}
	if entity != string(target.NamespaceArtwork) && entity != string(target.NamespaceNovel) {
		return errors.New("type must be one of: artwork, novel")
	}
	if entity == string(target.NamespaceNovel) && cmd.Flags().Changed("content-type") {
		return errors.New("--content-type is only supported when --type artwork")
	}
	if entity == string(target.NamespaceArtwork) && opts.artworkType != string(pixiv.ArtworkCategoryIllust) && opts.artworkType != string(pixiv.ArtworkCategoryManga) {
		return errors.New("content-type must be one of: illust, manga")
	}
	plan, request, jsonOut, ndjson, err := a.resolve(cmd, opts)
	if err != nil {
		return err
	}
	if entity == string(target.NamespaceNovel) {
		fetch := func(ctx context.Context, _ int64, client *pixiv.Client, cursor sdk.Cursor) ([]legacysdk.Novel, sdk.Cursor, error) {
			result, err := client.LatestNovels(ctx, pixiv.LatestNovelsOptions{Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			items := make([]legacysdk.Novel, len(result.Items))
			for i, item := range result.Items {
				items[i], err = record.NovelOutput(item)
				if err != nil {
					return nil, sdk.Cursor{}, err
				}
			}
			return items, result.Cursor, nil
		}
		return record.RunSDKList(cmd.Context(), a.env.Out, a.env.Err, a.exec, request, plan, record.FormatOf(jsonOut, ndjson), record.List[legacysdk.Novel]{
			RecordValue: func(item legacysdk.Novel) (any, error) { return record.RecordFromNovel(item) },
			Print:       func(items []legacysdk.Novel, _ int) error { return display.PrintNovels(a.env.Out, items) },
		}, fetch)
	}
	fetch := func(ctx context.Context, _ int64, client *pixiv.Client, cursor sdk.Cursor) ([]legacysdk.Artwork, sdk.Cursor, error) {
		result, err := client.LatestArtworks(ctx, pixiv.LatestArtworksOptions{Category: pixiv.ArtworkCategory(opts.artworkType), Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		items := make([]legacysdk.Artwork, len(result.Items))
		for i, item := range result.Items {
			items[i], err = record.ArtworkOutput(item, false)
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
		}
		return items, result.Cursor, nil
	}
	return record.RunSDKList(cmd.Context(), a.env.Out, a.env.Err, a.exec, request, plan, record.FormatOf(jsonOut, ndjson), record.List[legacysdk.Artwork]{
		RecordValue: func(item legacysdk.Artwork) (any, error) { return record.RecordFromArtwork(item) },
		Print:       func(items []legacysdk.Artwork, _ int) error { return display.PrintArtworks(a.env.Out, items) },
	}, fetch)
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

func filterFollowingArtworks(items []legacysdk.Artwork, filter searchfilter.Filter) []legacysdk.Artwork {
	if filter.ContentType == "" || filter.ContentType == searchfilter.ContentTypeAll {
		return items
	}
	filtered := make([]legacysdk.Artwork, 0, len(items))
	for _, item := range items {
		if filter.Matches(item.XRestrict, string(item.ContentType)) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}
