// Package mypixiv owns the authenticated `pixiv mypixiv` command group.
package mypixiv

import (
	"context"
	"errors"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/input"
	pixivcmd "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivcmd"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

// options 是只读 MyPixiv 命令的共同选项。MyPixiv 的好友关系写操作不在此命令组
// 中，避免把读取工作流与关系变更混在一起。
type options struct {
	command.Options
	ndjson      bool
	limit       int
	page        int
	contentType string
}

type commandRunner struct {
	env command.Env
	run pixivcmd.Runner
}

// New builds the actual `pixiv mypixiv` command group.
func New(env command.Env, exec pixivcmd.Executor) *cobra.Command {
	a := commandRunner{env: env, run: pixivcmd.New(env.Out, exec)}
	cmd := &cobra.Command{Use: "mypixiv", Short: "Browse authenticated MyPixiv data"}
	cmd.AddCommand(a.newUsersCommand(), a.newWorksCommand())
	env.BindNoInput(cmd)
	command.Bind(cmd, command.PixivData())
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
	pixivcmd.BindNDJSONFlag(cmd, &opts.ndjson)
	pixivcmd.BindListFlags(cmd, &opts.limit, &opts.page)
	a.env.BindNoInput(cmd)
	command.Bind(cmd, command.PixivData())
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
	pixivcmd.BindNDJSONFlag(cmd, &opts.ndjson)
	pixivcmd.BindListFlags(cmd, &opts.limit, &opts.page)
	cmd.Flags().StringVarP(&opts.contentType, "type", "t", "", "required entity type: artwork or novel")
	a.env.BindTextValue(cmd, 0, 1, 0)
	command.Bind(cmd, command.PixivData())
	return cmd
}

func (a commandRunner) runUsers(cmd *cobra.Command, opts options) error {
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
	return a.run.RunUsers(cmd.Context(), pixivcmd.Request(request), plan, pixivcmd.FormatOf(jsonOut, opts.ndjson), pixivcmd.UserList{Heading: func() string {
		return fmt.Sprintf("MyPixiv users for %d", userID)
	}, Fetch: fetch, Print: func(items []pixiv.UserPreview) error { return pixivcmd.PrintUserPreviews(a.env.Out, items) }})
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
	// 保留 illust 输入兼容既有脚本，但帮助与文档统一使用 artwork。
	if opts.contentType == "artwork" {
		opts.contentType = "illust"
	}
	var userID int64
	if len(args) == 0 {
		if opts.contentType != "illust" && opts.contentType != "novel" {
			return sdk.NewError("pixiv", "mypixiv works", sdk.InvalidArgument,
				sdk.WithDetail("type without USER_ID must be one of: artwork, novel"))
		}
	} else {
		if opts.contentType != "illust" && opts.contentType != "manga" && opts.contentType != "novel" {
			return sdk.NewError("pixiv", "mypixiv works", sdk.InvalidArgument,
				sdk.WithDetail("type with USER_ID must be one of: artwork, manga, novel"))
		}
		var err error
		userID, err = input.PositiveInt64(args[0], "user_id")
		if err != nil {
			return sdk.NewError("pixiv", "mypixiv works", sdk.InvalidArgument,
				sdk.WithDetail("user_id must be a positive integer"))
		}
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
	if len(args) == 0 {
		if opts.contentType == "illust" {
			fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
				result, err := client.MyPixivArtworks(ctx, pixiv.MyPixivArtworksRequest{Cursor: cursor})
				if err != nil {
					return nil, sdk.Cursor{}, err
				}
				return result.Items, result.Next, nil
			}
			return a.run.RunArtworks(cmd.Context(), pixivcmd.Request(request), plan, pixivcmd.FormatOf(jsonOut, ndjson), pixivcmd.ArtworkList{Heading: pixivcmd.Static("MyPixiv artworks"), Fetch: fetch, Print: func(items []pixiv.Artwork, start int) error { return pixivcmd.PrintArtworks(a.env.Out, items) }})
		}
		fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
			result, err := client.MyPixivNovels(ctx, pixiv.MyPixivNovelsRequest{Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			return result.Items, result.Next, nil
		}
		return a.run.RunNovels(cmd.Context(), pixivcmd.Request(request), plan, pixivcmd.FormatOf(jsonOut, ndjson), pixivcmd.NovelList{Heading: "MyPixiv novels", Fetch: fetch, Print: func(items []pixiv.Novel) error { return pixivcmd.PrintNovels(a.env.Out, items) }})
	}
	if opts.contentType == "novel" {
		fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
			result, err := client.UserNovels(ctx, pixiv.UserNovelsRequest{UserID: userID, Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			return result.Items, result.Next, nil
		}
		return a.run.RunNovels(cmd.Context(), pixivcmd.Request(request), plan, pixivcmd.FormatOf(jsonOut, ndjson), pixivcmd.NovelList{Heading: fmt.Sprintf("novels by %d", userID), Fetch: fetch, Print: func(items []pixiv.Novel) error { return pixivcmd.PrintNovels(a.env.Out, items) }})
	}
	fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		result, err := client.UserArtworks(ctx, pixiv.UserArtworksRequest{UserID: userID, Kind: pixiv.ArtworkKind(opts.contentType), Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return a.run.RunArtworks(cmd.Context(), pixivcmd.Request(request), plan, pixivcmd.FormatOf(jsonOut, ndjson), pixivcmd.ArtworkList{Heading: pixivcmd.Static(fmt.Sprintf("artworks by %d", userID)), Fetch: fetch, Print: func(items []pixiv.Artwork, start int) error { return pixivcmd.PrintArtworks(a.env.Out, items) }})
}
