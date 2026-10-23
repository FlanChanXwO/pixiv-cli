// Package recommended owns personalized Pixiv recommendation output.
package recommended

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	commands "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"

	pixivcmd "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivcmd"
	record "github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/internal/searchfilter"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

type options struct {
	commands.Options
	ndjson      bool
	limit       int
	page        int
	typ         string
	contentType string
}

type command struct {
	env    commands.Env
	runner pixivcmd.Runner
}

// New builds the actual `pixiv recommended` command.
func New(env commands.Env, exec pixivcmd.Executor) *cobra.Command {
	a := command{env: env, runner: pixivcmd.New(env.Out, exec)}
	opts := options{contentType: "all"}
	cmd := &cobra.Command{
		Use:   "recommended [KIND]",
		Short: "Show personalized recommendations",
		Args:  env.MaxArgs(1, "pixiv recommended [KIND] [options]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			kind, err := resolveKind(cmd, args, &opts, a.env.UsageError)
			if err != nil {
				return err
			}
			return a.execute(cmd, kind, opts)
		},
	}
	env.BindCommonFlags(cmd, &opts.Options)
	pixivcmd.BindNDJSONFlag(cmd, &opts.ndjson)
	pixivcmd.BindListFlags(cmd, &opts.limit, &opts.page)
	cmd.Flags().StringVarP(&opts.typ, "type", "t", opts.typ, "entity type: artwork, novel, user, all")
	cmd.Flags().StringVar(&opts.contentType, "content-type", opts.contentType, "local artwork subtype filter applied after recommendation pagination: all, illust, manga")
	env.BindTextValue(cmd, 0, 1, 0)
	commands.Bind(cmd, commands.PixivData())
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
	if cmd.Flags().Changed("type") && kind != "artwork" && kind != "novel" && kind != "user" && kind != "all" {
		return "", usage(errors.New("type must be one of artwork, novel, user, all"))
	}
	if cmd.Flags().Changed("content-type") {
		if kind != "artwork" {
			return "", usage(errors.New("--content-type is only supported when --type artwork"))
		}
		if opts.contentType != "all" && opts.contentType != "illust" && opts.contentType != "manga" {
			return "", usage(errors.New("content-type must be one of all, illust, manga"))
		}
	}
	if kind == "artwork" {
		if opts.contentType == "manga" {
			return "manga", nil
		}
		return "illust", nil
	}
	return kind, nil
}

func (a command) execute(cmd *cobra.Command, kind string, opts options) error {
	if !validKind(kind) {
		return errors.New("recommendation kind must be one of: all, illust, manga, novel, user")
	}
	plan, err := pixivcmd.ParsePlan(cmd, opts.limit, opts.page)
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
	if kind == "all" {
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
	if kind == "illust" || kind == "manga" {
		artworkFilter, err = resolveArtworkFilter(kind, opts.contentType)
		if err != nil {
			return err
		}
	}
	return a.runOne(cmd.Context(), request, plan, jsonOut, ndjson, kind, artworkFilter)
}

func validKind(kind string) bool {
	return kind == "all" || kind == "illust" || kind == "manga" || kind == "novel" || kind == "user"
}

func (a command) runOne(ctx context.Context, request pixivcmd.Request, plan pixivcmd.Plan, jsonOut, ndjson bool, kind string, artworkFilter searchfilter.Filter) error {
	if kind == "illust" || kind == "manga" {
		jsonKey := "illusts"
		if kind == "manga" {
			jsonKey = "manga"
		}
		fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
			result, err := client.RecommendedArtworks(ctx, pixiv.RecommendedArtworksRequest{Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			return result.Items, result.Next, nil
		}
		return a.runner.RunArtworks(ctx, request, plan, pixivcmd.FormatOf(jsonOut, ndjson), pixivcmd.ArtworkList{Key: pixivcmd.JSONKey(jsonKey), Heading: func() string {
			return fmt.Sprintf("recommended %s", kind)
		}, Fetch: fetch, PostFilter: func(items []pixiv.Artwork) []pixiv.Artwork {
			return filterRecommendedArtworks(items, artworkFilter)
		}, Print: func(items []pixiv.Artwork, start int) error { return pixivcmd.PrintArtworks(a.env.Out, items) }})
	}
	if kind == "novel" {
		fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
			result, err := client.RecommendedNovels(ctx, pixiv.RecommendedNovelsRequest{Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			return result.Items, result.Next, nil
		}
		return a.runner.RunNovels(ctx, request, plan, pixivcmd.FormatOf(jsonOut, ndjson), pixivcmd.NovelList{Heading: "recommended novels", Fetch: fetch, Print: func(items []pixiv.Novel) error { return pixivcmd.PrintNovels(a.env.Out, items) }})
	}
	fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error) {
		result, err := client.RecommendedUsers(ctx, pixiv.RecommendedUsersRequest{Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return a.runner.RunUsers(ctx, request, plan, pixivcmd.FormatOf(jsonOut, ndjson), pixivcmd.UserList{Heading: func() string { return "recommended users" }, Fetch: fetch, Print: func(items []pixiv.UserPreview) error { return pixivcmd.PrintUserPreviews(a.env.Out, items) }})
}

func resolveArtworkFilter(kind, contentType string) (searchfilter.Filter, error) {
	if kind == "manga" {
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
		if filter.Matches(0, string(item.Kind)) {
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
func (a command) runAllNDJSON(ctx context.Context, client *pixiv.Client, plan pixivcmd.Plan) (bool, error) {
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
	if err := pixivcmd.PageItems(ctx, plan, fetchRecommendedArtworks(client, searchfilter.Filter{}), func(items []pixiv.Artwork) error {
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
	if err := pixivcmd.PageItems(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
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
	err := pixivcmd.PageItems(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error) {
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
