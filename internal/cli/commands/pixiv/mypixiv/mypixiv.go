// Package mypixiv owns the authenticated `pixiv mypixiv` command group.
package mypixiv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	requirements "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands"
	deps "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/internal/listing"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/parse"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

// options 是只读 MyPixiv 命令的共同选项。MyPixiv 的好友关系写操作不在此命令组
// 中，避免把读取工作流与关系变更混在一起。
type options struct {
	deps.CommandOptions
	ndjson      bool
	limit       int
	page        int
	contentType string
}

type command struct {
	data deps.Data
}

// New builds the actual `pixiv mypixiv` command group.
func New(data deps.Data) *cobra.Command {
	a := command{data: data}
	cmd := &cobra.Command{Use: "mypixiv", Short: "Browse authenticated MyPixiv data"}
	cmd.AddCommand(a.newUsersCommand(), a.newWorksCommand())
	data.BindNoInput(cmd)
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

func (a command) newUsersCommand() *cobra.Command {
	opts := options{}
	cmd := &cobra.Command{
		Use:   "users",
		Short: "List MyPixiv users for the authenticated account",
		Args:  a.data.ExactArgs(0, "pixiv mypixiv users"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.runUsers(cmd, opts)
		},
	}
	a.data.BindCommonFlags(cmd, &opts.CommandOptions)
	listing.BindNDJSONFlag(cmd, &opts.ndjson)
	listing.BindListFlags(cmd, &opts.limit, &opts.page)
	a.data.BindNoInput(cmd)
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

func (a command) newWorksCommand() *cobra.Command {
	opts := options{}
	cmd := &cobra.Command{
		Use:   "works [USER_ID]",
		Short: "Browse MyPixiv works or one user's works",
		Args:  a.data.MaxArgs(1, "pixiv mypixiv works [USER_ID] --type artwork|manga|novel"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runWorks(cmd, args, opts)
		},
	}
	a.data.BindCommonFlags(cmd, &opts.CommandOptions)
	listing.BindNDJSONFlag(cmd, &opts.ndjson)
	listing.BindListFlags(cmd, &opts.limit, &opts.page)
	cmd.Flags().StringVarP(&opts.contentType, "type", "t", "", "required entity type: artwork or novel")
	a.data.BindTextValue(cmd, 0, 1, 0)
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

func (a command) runUsers(cmd *cobra.Command, opts options) error {
	plan, err := listing.ParsePlan(cmd, opts.limit, opts.page)
	if err != nil {
		return err
	}
	request, err := a.data.Request(cmd, opts.CommandOptions)
	if err != nil {
		return err
	}
	jsonOverride := a.data.JSONOverride(cmd, opts.CommandOptions)
	if opts.ndjson && cmd.Flags().Changed("json") {
		return a.data.Usage(errors.New("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !opts.ndjson {
		jsonOut, err = a.data.JSONOut(jsonOverride)
		if err != nil {
			return err
		}
	}
	var userID int64
	fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error) {
		if userID == 0 {
			var err error
			userID, err = deps.CurrentUserID(client)
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
	return a.runner().RunPooledUserList(cmd.Context(), listing.Request(request), plan, jsonOut, opts.ndjson, func() string {
		return fmt.Sprintf("MyPixiv users for %d", userID)
	}, fetch, func(items []pixiv.UserPreview) error { return printUserPreviews(a.data.Output, items) })
}

func (a command) runWorks(cmd *cobra.Command, args []string, opts options) error {
	// CLI 的 --type 选择实体，公开名称 artwork 对应 App API 的 illust 子类型。
	// 保留 illust 输入兼容既有脚本，但帮助与文档统一使用 artwork。
	if opts.contentType == "artwork" {
		opts.contentType = "illust"
	}
	var userID int64
	if len(args) == 0 {
		if opts.contentType != "illust" && opts.contentType != "novel" {
			return errors.New("type without USER_ID must be one of: artwork, novel")
		}
	} else {
		if opts.contentType != "illust" && opts.contentType != "manga" && opts.contentType != "novel" {
			return errors.New("type with USER_ID must be one of: artwork, manga, novel")
		}
		var err error
		userID, err = parse.PositiveInt64(args[0], "user_id")
		if err != nil {
			return err
		}
	}
	plan, err := listing.ParsePlan(cmd, opts.limit, opts.page)
	if err != nil {
		return err
	}
	request, err := a.data.Request(cmd, opts.CommandOptions)
	if err != nil {
		return err
	}
	jsonOverride := a.data.JSONOverride(cmd, opts.CommandOptions)
	if opts.ndjson && cmd.Flags().Changed("json") {
		return a.data.Usage(errors.New("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !opts.ndjson {
		jsonOut, err = a.data.JSONOut(jsonOverride)
		if err != nil {
			return err
		}
	}
	ndjson := a.data.ShouldAutoNDJSON(cmd, opts.ndjson, jsonOut)
	if len(args) == 0 {
		if opts.contentType == "illust" {
			fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
				result, err := client.MyPixivArtworks(ctx, pixiv.MyPixivArtworksRequest{Cursor: cursor})
				if err != nil {
					return nil, sdk.Cursor{}, err
				}
				return result.Items, result.Next, nil
			}
			return a.runner().RunPooledIllustList(cmd.Context(), listing.Request(request), plan, jsonOut, ndjson, "MyPixiv artworks", fetch,
				func(items []pixiv.Artwork, start int) error { return printArtworks(a.data.Output, items) })
		}
		fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
			result, err := client.MyPixivNovels(ctx, pixiv.MyPixivNovelsRequest{Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			return result.Items, result.Next, nil
		}
		return a.runner().RunPooledNovelList(cmd.Context(), listing.Request(request), plan, jsonOut, ndjson, "MyPixiv novels", fetch,
			func(items []pixiv.Novel) error { return printNovels(a.data.Output, items) })
	}
	if opts.contentType == "novel" {
		fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
			result, err := client.UserNovels(ctx, pixiv.UserNovelsRequest{UserID: userID, Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			return result.Items, result.Next, nil
		}
		return a.runner().RunPooledNovelList(cmd.Context(), listing.Request(request), plan, jsonOut, ndjson, fmt.Sprintf("novels by %d", userID), fetch,
			func(items []pixiv.Novel) error { return printNovels(a.data.Output, items) })
	}
	fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		result, err := client.UserArtworks(ctx, pixiv.UserArtworksRequest{UserID: userID, Kind: pixiv.ArtworkKind(opts.contentType), Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return a.runner().RunPooledIllustList(cmd.Context(), listing.Request(request), plan, jsonOut, ndjson, fmt.Sprintf("artworks by %d", userID), fetch,
		func(items []pixiv.Artwork, start int) error { return printArtworks(a.data.Output, items) })
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

func printUserPreviews(out io.Writer, users []pixiv.UserPreview) error {
	for _, item := range users {
		if _, err := fmt.Fprintf(out, "%d %s\n", item.User.ID, item.User.Name); err != nil {
			return err
		}
	}
	return nil
}
