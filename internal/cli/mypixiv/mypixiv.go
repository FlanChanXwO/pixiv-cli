// Package mypixiv 提供 pixiv mypixiv 命令组，浏览当前账号的 MyPixiv 用户与作品。
package mypixiv

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/display"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/target"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

// options 保存只读 MyPixiv 命令的共同选项。MyPixiv 的好友关系写操作不在此命令组
// 中，避免把读取工作流与关系变更混在一起。
type options struct {
	environment.Options
	ndjson      bool
	limit       int
	page        int
	contentType string
}

type commandRunner struct {
	env environment.Env
	run session.Runner
}

// New 构造 `pixiv mypixiv` 命令组；用户与作品两个子命令共用同一个执行端口，
// 但各自执行时都会重新选择账号。
func New(env environment.Env, exec session.Executor) *cobra.Command {
	a := commandRunner{env: env, run: session.NewRunner(env.Out, exec)}
	cmd := &cobra.Command{Use: "mypixiv", Short: "Browse authenticated MyPixiv data"}
	cmd.AddCommand(a.newUsersCommand(), a.newWorksCommand())
	env.BindNoInput(cmd)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a commandRunner) newUsersCommand() *cobra.Command {
	opts := options{}
	cmd := &cobra.Command{
		Use:   "users",
		Short: "List MyPixiv users for the authenticated account",
		Args:  a.env.ExactArgs(0, "pixiv mypixiv users"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.runUsers(cmd, opts)
		},
	}
	a.env.BindCommonFlags(cmd, &opts.Options)
	environment.BindNDJSONFlag(cmd, &opts.ndjson)
	environment.BindListFlags(cmd, &opts.limit, &opts.page)
	a.env.BindNoInput(cmd)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a commandRunner) newWorksCommand() *cobra.Command {
	opts := options{}
	cmd := &cobra.Command{
		Use:   "works [USER_ID]",
		Short: "Browse MyPixiv works or one user's works",
		Args:  a.env.MaxArgs(1, "pixiv mypixiv works [USER_ID] --type artwork|manga|novel"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runWorks(cmd, args, opts)
		},
	}
	a.env.BindCommonFlags(cmd, &opts.Options)
	environment.BindNDJSONFlag(cmd, &opts.ndjson)
	environment.BindListFlags(cmd, &opts.limit, &opts.page)
	cmd.Flags().StringVarP(&opts.contentType, "type", "t", "", "required entity type: artwork or novel")
	a.env.BindTextValue(cmd, 0, 1, 0)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a commandRunner) runUsers(cmd *cobra.Command, opts options) error {
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
	var userID int64
	fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error) {
		if userID == 0 {
			var err error
			userID, err = currentUserID(client)
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
		}
		result, err := client.MyPixivUsers(ctx, pixiv.MyPixivUsersRequest{Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return record.RunUsers(cmd.Context(), a.run, request, plan, record.FormatOf(jsonOut, opts.ndjson), a.env.Err, record.UserList{Fetch: fetch, Print: func(items []pixiv.UserPreview) error { return display.PrintUserPreviews(a.env.Out, items) }})
}

func currentUserID(client *pixiv.Client) (int64, error) {
	if id := client.UserID(); id > 0 {
		return id, nil
	}
	return 0, sdk.NewError("pixiv", "MyPixivUsers", sdk.Unauthorized,
		sdk.WithDetail("cannot determine current user id"))
}

func (a commandRunner) runWorks(cmd *cobra.Command, args []string, opts options) error {
	// CLI 的 --type 选择实体，公开名称 artwork 对应 App API 的 illust 子类型。
	if opts.contentType == string(target.ResultKindArtwork) {
		opts.contentType = string(target.SubtypeIllust)
	}
	var userID int64
	if len(args) == 0 {
		if opts.contentType != string(target.SubtypeIllust) && opts.contentType != string(target.ResultKindNovel) {
			return sdk.NewError("pixiv", "mypixiv works", sdk.InvalidArgument,
				sdk.WithDetail("type without USER_ID must be one of: artwork, novel"))
		}
	} else {
		if opts.contentType != string(target.SubtypeIllust) && opts.contentType != string(target.SubtypeManga) && opts.contentType != string(target.ResultKindNovel) {
			return sdk.NewError("pixiv", "mypixiv works", sdk.InvalidArgument,
				sdk.WithDetail("type with USER_ID must be one of: artwork, manga, novel"))
		}
		var err error
		userID, err = environment.PositiveInt64(args[0], "user_id")
		if err != nil {
			return sdk.NewError("pixiv", "mypixiv works", sdk.InvalidArgument,
				sdk.WithDetail("user_id must be a positive integer"))
		}
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
	if len(args) == 0 {
		if opts.contentType == string(target.SubtypeIllust) {
			fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
				result, err := client.MyPixivArtworks(ctx, pixiv.MyPixivArtworksRequest{Cursor: cursor})
				if err != nil {
					return nil, sdk.Cursor{}, err
				}
				return result.Items, result.Next, nil
			}
			return record.RunArtworks(cmd.Context(), a.run, request, plan, record.FormatOf(jsonOut, ndjson), a.env.Err, record.ArtworkList{Fetch: fetch, Print: func(items []pixiv.Artwork, start int) error { return display.PrintArtworks(a.env.Out, items) }})
		}
		fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
			result, err := client.MyPixivNovels(ctx, pixiv.MyPixivNovelsRequest{Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			return result.Items, result.Next, nil
		}
		return record.RunNovels(cmd.Context(), a.run, request, plan, record.FormatOf(jsonOut, ndjson), a.env.Err, record.NovelList{Fetch: fetch, Print: func(items []pixiv.Novel) error { return display.PrintNovels(a.env.Out, items) }})
	}
	if opts.contentType == string(target.ResultKindNovel) {
		fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
			result, err := client.UserNovels(ctx, pixiv.UserNovelsRequest{UserID: userID, Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			return result.Items, result.Next, nil
		}
		return record.RunNovels(cmd.Context(), a.run, request, plan, record.FormatOf(jsonOut, ndjson), a.env.Err, record.NovelList{Fetch: fetch, Print: func(items []pixiv.Novel) error { return display.PrintNovels(a.env.Out, items) }})
	}
	fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		result, err := client.UserArtworks(ctx, pixiv.UserArtworksRequest{UserID: userID, Kind: pixiv.ArtworkKind(opts.contentType), Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return record.RunArtworks(cmd.Context(), a.run, request, plan, record.FormatOf(jsonOut, ndjson), a.env.Err, record.ArtworkList{Fetch: fetch, Print: func(items []pixiv.Artwork, start int) error { return display.PrintArtworks(a.env.Out, items) }})
}
