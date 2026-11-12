// Package recommended 提供 pixiv recommended 命令，输出个性化推荐的作品、小说与用户。
//
// 包的组成：[New] 组装命令；all.go 暂存文本和 JSON 分区；NDJSON 按类别顺序提交。
package recommended

import (
	"context"
	"encoding/json"
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
	env  environment.Env
	exec session.SDKExecutor
}

// New 构造 `pixiv recommended` 命令，保留各输出格式的提交时机。
func New(env environment.Env, exec session.SDKExecutor) *cobra.Command {
	a := commandRunner{env: env, exec: exec}
	opts := options{contentType: string(pixiv.ArtworkCategoryAll)}
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
		if opts.contentType != string(pixiv.ArtworkCategoryAll) && opts.contentType != string(pixiv.ArtworkCategoryIllust) && opts.contentType != string(pixiv.ArtworkCategoryManga) {
			return "", usage(errors.New("content-type must be one of all, illust, manga"))
		}
	}
	if kind == string(target.NamespaceArtwork) {
		if opts.contentType == string(pixiv.ArtworkCategoryManga) {
			return string(pixiv.ArtworkCategoryManga), nil
		}
		return string(pixiv.ArtworkCategoryIllust), nil
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
			return a.exec(cmd.Context(), request, func(ctx context.Context, _ int64, client *pixiv.Client) (bool, error) {
				return a.runAllNDJSON(ctx, client, plan)
			})
		}
		return a.exec(cmd.Context(), request, func(ctx context.Context, _ int64, client *pixiv.Client) (bool, error) {
			return a.runAll(ctx, client, plan, jsonOut)
		})
	}
	artworkFilter := searchfilter.Filter{}
	if kind == string(pixiv.ArtworkCategoryIllust) || kind == string(pixiv.ArtworkCategoryManga) {
		artworkFilter, err = resolveArtworkFilter(kind, opts.contentType)
		if err != nil {
			return err
		}
	}
	return a.runOne(cmd.Context(), request, plan, jsonOut, ndjson, kind, artworkFilter)
}

func validKind(kind string) bool {
	return kind == recommendationKindAll || kind == string(pixiv.ArtworkCategoryIllust) || kind == string(pixiv.ArtworkCategoryManga) || kind == string(target.NamespaceNovel) || kind == string(target.NamespaceUser)
}

func (a commandRunner) runOne(ctx context.Context, request session.ProxyRequest, plan pagination.PagePlan, jsonOut, ndjson bool, kind string, artworkFilter searchfilter.Filter) error {
	format := record.FormatOf(jsonOut, ndjson)
	if kind == string(pixiv.ArtworkCategoryIllust) || kind == string(pixiv.ArtworkCategoryManga) {
		return record.RunSDKList(ctx, a.env.Out, a.env.Err, a.exec, request, plan, format, record.List[legacysdk.Artwork]{
			PostFilter: func(items []legacysdk.Artwork) []legacysdk.Artwork {
				return filterRecommendedArtworks(items, artworkFilter)
			},
			Print:       func(items []legacysdk.Artwork, _ int) error { return display.PrintArtworks(a.env.Out, items) },
			RecordValue: func(item legacysdk.Artwork) (any, error) { return record.RecordFromArtwork(item) },
		}, func(ctx context.Context, _ int64, client *pixiv.Client, cursor sdk.Cursor) ([]legacysdk.Artwork, sdk.Cursor, error) {
			return fetchRecommendedArtworks(client)(ctx, cursor)
		})
	}
	if kind == string(target.NamespaceNovel) {
		return record.RunSDKList(ctx, a.env.Out, a.env.Err, a.exec, request, plan, format, record.List[legacysdk.Novel]{
			Print:       func(items []legacysdk.Novel, _ int) error { return display.PrintNovels(a.env.Out, items) },
			RecordValue: func(item legacysdk.Novel) (any, error) { return record.RecordFromNovel(item) },
		}, func(ctx context.Context, _ int64, client *pixiv.Client, cursor sdk.Cursor) ([]legacysdk.Novel, sdk.Cursor, error) {
			return fetchRecommendedNovels(client)(ctx, cursor)
		})
	}
	return record.RunSDKList(ctx, a.env.Out, a.env.Err, a.exec, request, plan, format, record.List[legacysdk.UserPreview]{
		Print:       func(items []legacysdk.UserPreview, _ int) error { return display.PrintUserPreviews(a.env.Out, items) },
		RecordValue: func(item legacysdk.UserPreview) (any, error) { return record.RecordFromUserPreview(item) },
	}, func(ctx context.Context, _ int64, client *pixiv.Client, cursor sdk.Cursor) ([]legacysdk.UserPreview, sdk.Cursor, error) {
		return fetchRecommendedUsers(client)(ctx, cursor)
	})
}

func resolveArtworkFilter(kind, contentType string) (searchfilter.Filter, error) {
	if kind == string(pixiv.ArtworkCategoryManga) {
		contentType = string(searchfilter.ContentTypeManga)
	}
	return searchfilter.NormalizeFilter("", contentType)
}

func filterRecommendedArtworks(items []legacysdk.Artwork, filter searchfilter.Filter) []legacysdk.Artwork {
	if filter.ContentType == "" || filter.ContentType == searchfilter.ContentTypeAll {
		return items
	}
	filtered := make([]legacysdk.Artwork, 0, len(items))
	for _, item := range items {
		if filter.Matches(0, string(item.ContentType)) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func fetchRecommendedArtworks(client *pixiv.Client) func(context.Context, sdk.Cursor) ([]legacysdk.Artwork, sdk.Cursor, error) {
	return func(ctx context.Context, cursor sdk.Cursor) ([]legacysdk.Artwork, sdk.Cursor, error) {
		result, err := client.RecommendedArtworks(ctx, pixiv.RecommendedArtworksOptions{Cursor: cursor})
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
}

func fetchRecommendedNovels(client *pixiv.Client) func(context.Context, sdk.Cursor) ([]legacysdk.Novel, sdk.Cursor, error) {
	return func(ctx context.Context, cursor sdk.Cursor) ([]legacysdk.Novel, sdk.Cursor, error) {
		result, err := client.RecommendedNovels(ctx, pixiv.RecommendedNovelsOptions{Cursor: cursor})
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
}

func fetchRecommendedUsers(client *pixiv.Client) func(context.Context, sdk.Cursor) ([]legacysdk.UserPreview, sdk.Cursor, error) {
	return func(ctx context.Context, cursor sdk.Cursor) ([]legacysdk.UserPreview, sdk.Cursor, error) {
		result, err := client.RecommendedUsers(ctx, pixiv.RecommendedUsersOptions{Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		items := make([]legacysdk.UserPreview, len(result.Items))
		for i, item := range result.Items {
			items[i], err = record.RecommendationOutput(item)
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
		}
		return items, result.Cursor, nil
	}
}

// runAllNDJSON 保留 all 的既有类别顺序。每写出一条记录即标记提交，因此账号池只会
// 在任何下游可见输出之前重放 429。
func (a commandRunner) runAllNDJSON(ctx context.Context, client *pixiv.Client, plan pagination.PagePlan) (bool, error) {
	encoder := json.NewEncoder(a.env.Out)
	committed := false
	writeArtworks := func(items []legacysdk.Artwork) error {
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
	var visualItems []legacysdk.Artwork
	if err := pagination.PageItems(ctx, plan, fetchRecommendedArtworks(client), func(items []legacysdk.Artwork) error {
		visualItems = append(visualItems, items...)
		return nil
	}); err != nil {
		return committed, err
	}
	if err := writeArtworks(visualItems); err != nil {
		return committed, err
	}

	if err := pagination.PageItems(ctx, plan, fetchRecommendedNovels(client), func(items []legacysdk.Novel) error {
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
	err := pagination.PageItems(ctx, plan, fetchRecommendedUsers(client), func(items []legacysdk.UserPreview) error {
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
