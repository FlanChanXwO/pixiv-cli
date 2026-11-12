// Package mypixiv 提供 pixiv mypixiv 命令组，浏览当前账号的 MyPixiv 用户与作品。
package mypixiv

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/display"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	legacysdk "github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk/pixiv"
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
	env  environment.Env
	exec session.SDKExecutor
}

// New 构造 `pixiv mypixiv` 命令组；用户与作品两个子命令共用同一个执行端口，
// 但各自执行时都会重新选择账号。
func New(env environment.Env, exec session.SDKExecutor) *cobra.Command {
	a := commandRunner{env: env, exec: exec}
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
	fetch := func(ctx context.Context, _ int64, client *pixiv.Client, cursor sdk.Cursor) ([]legacysdk.UserPreview, sdk.Cursor, error) {
		result, err := client.MyPixivUsers(ctx, pixiv.MyPixivOptions{Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		items := make([]legacysdk.UserPreview, len(result.Items))
		for i, item := range result.Items {
			items[i], err = record.UserPreviewOutput(item)
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
		}
		return items, result.Cursor, nil
	}
	return record.RunSDKList(cmd.Context(), a.env.Out, a.env.Err, a.exec, request, plan, record.FormatOf(jsonOut, opts.ndjson), record.List[legacysdk.UserPreview]{
		RecordValue: func(item legacysdk.UserPreview) (any, error) { return record.RecordFromUserPreview(item) },
		Print:       func(items []legacysdk.UserPreview, _ int) error { return display.PrintUserPreviews(a.env.Out, items) },
	}, fetch)
}

func (a commandRunner) runWorks(cmd *cobra.Command, args []string, opts options) error {
	// CLI 的 --type 选择实体，公开名称 artwork 对应 App API 的 illust 子类型。
	if opts.contentType == string(target.NamespaceArtwork) {
		opts.contentType = string(pixiv.ContentTypeIllust)
	}
	var userID int64
	if len(args) == 0 {
		if opts.contentType != string(pixiv.ContentTypeIllust) && opts.contentType != string(target.NamespaceNovel) {
			return &sdk.Error{Operation: "pixiv:mypixiv works", Reason: sdk.InvalidArgument, Message: "type without USER_ID must be one of: artwork, novel"}
		}
	} else {
		if opts.contentType != string(pixiv.ContentTypeIllust) && opts.contentType != string(pixiv.ContentTypeManga) && opts.contentType != string(target.NamespaceNovel) {
			return &sdk.Error{Operation: "pixiv:mypixiv works", Reason: sdk.InvalidArgument, Message: "type with USER_ID must be one of: artwork, manga, novel"}
		}
		var err error
		userID, err = environment.PositiveInt64(args[0], "user_id")
		if err != nil {
			return &sdk.Error{Operation: "pixiv:mypixiv works", Reason: sdk.InvalidArgument, Message: "user_id must be a positive integer"}
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
	if opts.contentType == string(target.NamespaceNovel) {
		fetch := func(ctx context.Context, _ int64, client *pixiv.Client, cursor sdk.Cursor) ([]legacysdk.Novel, sdk.Cursor, error) {
			var result sdk.Page[*pixiv.Novel]
			var err error
			if len(args) == 0 {
				result, err = client.MyPixivNovels(ctx, pixiv.MyPixivOptions{Cursor: cursor})
			} else {
				result, err = client.User(userID).Novels(ctx, pixiv.UserNovelsOptions{Cursor: cursor})
			}
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
		var result sdk.Page[*pixiv.Artwork]
		var err error
		if len(args) == 0 {
			result, err = client.MyPixivArtworks(ctx, pixiv.MyPixivOptions{Cursor: cursor})
		} else {
			result, err = client.User(userID).Artworks(ctx, pixiv.UserArtworksOptions{Kind: pixiv.ContentType(opts.contentType), Cursor: cursor})
		}
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
