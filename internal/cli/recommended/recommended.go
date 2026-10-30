// Package recommended 提供 pixiv recommended 命令，输出个性化推荐的作品或小说。
//
// 包的组成：[New] 组装命令；多段输出先在 spool.go 暂存，全部成功后再提交。
package recommended

import (
	"context"
	"encoding/json"
	"errors"
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

type options struct {
	environment.Options
	ndjson      bool
	limit       int
	page        int
	typ         string
	contentType string
}

// recommendationKindAll 标记“全部类型”的聚合选择，不会作为实体类型传给 SDK。
const recommendationKindAll = "all"

type commandRunner struct {
	env    environment.Env
	runner session.Runner
}

// New 构造 `pixiv recommended` 命令；--type all 需要多段输出，只在全部成功后才提交。
func New(env environment.Env, exec session.Executor) *cobra.Command {
	a := commandRunner{env: env, runner: session.NewRunner(env.Out, exec)}
	opts := options{contentType: string(pixiv.SearchContentTypeAll)}
	cmd := &cobra.Command{
		Use:   "recommended [KIND]",
		Short: "Show personalized recommendations",
		Args:  env.MaxArgs(1, "pixiv recommended [KIND] [options]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			kind, err := resolveKind(cmd, args, &opts, a.env.WrapUsage)
			if err != nil {
				return err
			}
			return a.execute(cmd, kind, opts)
		},
	}
	env.BindCommonFlags(cmd, &opts.Options)
	environment.BindNDJSONFlag(cmd, &opts.ndjson)
	environment.BindListFlags(cmd, &opts.limit, &opts.page)
	cmd.Flags().StringVarP(&opts.typ, "type", "t", opts.typ, "entity type: artwork, novel, user, all")
	cmd.Flags().StringVar(&opts.contentType, "content-type", opts.contentType, "local artwork subtype filter applied after recommendation pagination: all, illust, manga")
	env.BindTextValue(cmd, 0, 1, 0)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

// resolveKind 保留 KIND 位置参数与 --type 的既有互斥、取值域和 --content-type
// 组合规则；两种入口最终解析为同一组内部 kind。
func resolveKind(cmd *cobra.Command, args []string, opts *options, usage func(error) error) (string, error) {
	kind := opts.typ
	if len(args) == 1 {
		if cmd.Flags().Changed("type") {
			return "", usage(errors.New("KIND cannot be combined with --type"))
		}
		kind = args[0]
	}
	if kind == "" {
		return "", errors.New("recommended requires KIND or --type")
	}
	if cmd.Flags().Changed("type") && kind != string(target.NamespaceArtwork) && kind != string(target.NamespaceNovel) && kind != string(target.NamespaceUser) && kind != recommendationKindAll {
		return "", usage(errors.New("type must be one of artwork, novel, user, all"))
	}
	if cmd.Flags().Changed("content-type") {
		if kind != string(target.NamespaceArtwork) {
			return "", usage(errors.New("--content-type is only supported when --type artwork"))
		}
		if opts.contentType != string(pixiv.SearchContentTypeAll) && opts.contentType != string(pixiv.SearchContentTypeIllust) && opts.contentType != string(pixiv.SearchContentTypeManga) {
			return "", usage(errors.New("content-type must be one of all, illust, manga"))
		}
	}
	if kind == string(target.NamespaceArtwork) {
		if opts.contentType == string(pixiv.SearchContentTypeManga) {
			return string(pixiv.SearchContentTypeManga), nil
		}
		return string(pixiv.SearchContentTypeIllust), nil
	}
	return kind, nil
}

func (a commandRunner) execute(cmd *cobra.Command, kind string, opts options) error {
	if !validKind(kind) {
		return errors.New("recommendation kind must be one of: all, illust, manga, novel, user")
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
	if kind == recommendationKindAll {
		if ndjson {
			return a.runner.Execute(request)(cmd.Context(), func(ctx context.Context, client *pixiv.Client) (bool, error) {
				return a.runAllNDJSON(ctx, client, plan)
			})
		}
		return a.runner.Execute(request)(cmd.Context(), func(ctx context.Context, client *pixiv.Client) (bool, error) {
			return a.runAll(ctx, client, plan, jsonOut)
		})
	}
	artworkFilter := searchfilter.Filter{}
	if kind == string(pixiv.SearchContentTypeIllust) || kind == string(pixiv.SearchContentTypeManga) {
		artworkFilter, err = resolveArtworkFilter(kind, opts.contentType)
		if err != nil {
			return err
		}
	}
	return a.runOne(cmd.Context(), request, plan, jsonOut, ndjson, kind, artworkFilter)
}

func validKind(kind string) bool {
	return kind == recommendationKindAll || kind == string(pixiv.SearchContentTypeIllust) || kind == string(pixiv.SearchContentTypeManga) || kind == string(target.NamespaceNovel) || kind == string(target.NamespaceUser)
}

func (a commandRunner) runOne(ctx context.Context, request session.ProxyRequest, plan pagination.PagePlan, jsonOut, ndjson bool, kind string, artworkFilter searchfilter.Filter) error {
	if kind == string(pixiv.SearchContentTypeIllust) || kind == string(pixiv.SearchContentTypeManga) {
		jsonKey := string(record.JSONKeyIllusts)
		if kind == string(pixiv.SearchContentTypeManga) {
			jsonKey = string(record.JSONKeyManga)
		}
		fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
			result, err := client.RecommendedArtworks(ctx, pixiv.RecommendedArtworksRequest{Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			return result.Items, result.Next, nil
		}
		return record.RunArtworks(ctx, a.runner, request, plan, record.FormatOf(jsonOut, ndjson), a.env.Err, record.ArtworkList{Key: record.JSONKey(jsonKey), Fetch: fetch, PostFilter: func(items []pixiv.Artwork) []pixiv.Artwork {
			return filterRecommendedArtworks(items, artworkFilter)
		}, Print: func(items []pixiv.Artwork, start int) error { return display.PrintArtworks(a.env.Out, items) }})
	}
	if kind == string(target.NamespaceNovel) {
		fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
			result, err := client.RecommendedNovels(ctx, pixiv.RecommendedNovelsRequest{Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			return result.Items, result.Next, nil
		}
		return record.RunNovels(ctx, a.runner, request, plan, record.FormatOf(jsonOut, ndjson), a.env.Err, record.NovelList{Fetch: fetch, Print: func(items []pixiv.Novel) error { return display.PrintNovels(a.env.Out, items) }})
	}
	fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error) {
		result, err := client.RecommendedUsers(ctx, pixiv.RecommendedUsersRequest{Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return record.RunUsers(ctx, a.runner, request, plan, record.FormatOf(jsonOut, ndjson), a.env.Err, record.UserList{Fetch: fetch, Print: func(items []pixiv.UserPreview) error { return display.PrintUserPreviews(a.env.Out, items) }})
}

func resolveArtworkFilter(kind, contentType string) (searchfilter.Filter, error) {
	if kind == string(pixiv.SearchContentTypeManga) {
		contentType = string(searchfilter.ContentTypeManga)
	}
	return searchfilter.NormalizeFilter("", contentType)
}

func filterRecommendedArtworks(items []pixiv.Artwork, filter searchfilter.Filter) []pixiv.Artwork {
	if filter.ContentType == "" || filter.ContentType == searchfilter.ContentTypeAll {
		return items
	}
	filtered := make([]pixiv.Artwork, 0, len(items))
	for _, item := range items {
		if filter.Matches(0, string(item.ContentType)) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func fetchRecommendedArtworks(client *pixiv.Client, filter searchfilter.Filter) func(context.Context, sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
	return func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		result, err := client.RecommendedArtworks(ctx, pixiv.RecommendedArtworksRequest{Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return filterRecommendedArtworks(result.Items, filter), result.Next, nil
	}
}

// runAllNDJSON 保留 all 的既有类别顺序。每写出一条记录即标记提交，因此账号池只会
// 在任何下游可见输出之前重放 429。
func (a commandRunner) runAllNDJSON(ctx context.Context, client *pixiv.Client, plan pagination.PagePlan) (bool, error) {
	encoder := json.NewEncoder(a.env.Out)
	committed := false
	writeArtworks := func(items []pixiv.Artwork) error {
		for _, item := range items {
			record, err := record.RecordFromArtwork(item)
			if err != nil {
				return err
			}
			committed = true
			if err := encoder.Encode(record); err != nil {
				return err
			}
		}
		return nil
	}
	var visualItems []pixiv.Artwork
	if err := pagination.PageItems(ctx, plan, fetchRecommendedArtworks(client, searchfilter.Filter{}), func(items []pixiv.Artwork) error {
		visualItems = append(visualItems, items...)
		return nil
	}); err != nil {
		return committed, err
	}
	for _, filter := range []searchfilter.Filter{{ContentType: searchfilter.ContentTypeIllust}, {ContentType: searchfilter.ContentTypeManga}} {
		if err := writeArtworks(filterRecommendedArtworks(visualItems, filter)); err != nil {
			return committed, err
		}
	}
	if err := pagination.PageItems(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
		result, err := client.RecommendedNovels(ctx, pixiv.RecommendedNovelsRequest{Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}, func(items []pixiv.Novel) error {
		for _, item := range items {
			record, err := record.RecordFromNovel(item)
			if err != nil {
				return err
			}
			committed = true
			if err := encoder.Encode(record); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return committed, err
	}
	err := pagination.PageItems(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error) {
		result, err := client.RecommendedUsers(ctx, pixiv.RecommendedUsersRequest{Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}, func(items []pixiv.UserPreview) error {
		for _, item := range items {
			record, err := record.RecordFromUserPreview(item)
			if err != nil {
				return err
			}
			committed = true
			if err := encoder.Encode(record); err != nil {
				return err
			}
		}
		return nil
	})
	return committed, err
}
