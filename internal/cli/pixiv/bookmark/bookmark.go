// Package bookmark owns Pixiv bookmark reads and mutations.
package bookmark

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivdeps"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pipeline"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pixiv/internal/listing"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/requirements"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/parse"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

var visualRecordTypes = map[string]struct{}{
	"illust": {},
	"manga":  {},
	"ugoira": {},
}

type listOptions struct {
	deps.CommandOptions
	ndjson   bool
	limit    int
	page     int
	typ      string
	restrict string
	tag      string
}

type mutationOptions struct {
	deps.CommandOptions
	restrict string
	tags     []string
	onError  string
}

type command struct {
	data deps.Data
}

// New builds the actual `pixiv bookmark` group command.
func New(data deps.Data) *cobra.Command {
	a := command{data: data}
	cmd := &cobra.Command{Use: "bookmark", Short: "Manage illustration bookmarks"}
	cmd.AddCommand(a.newList(), a.newTags(), a.newDetail(), a.newAdd(), a.newRemove())
	data.BindNoInput(cmd)
	return cmd
}

func (a command) newList() *cobra.Command {
	opts := listOptions{typ: "artwork", restrict: string(pixiv.RestrictPublic)}
	cmd := &cobra.Command{
		Use:   "list [USER_ID]",
		Short: "List artwork or novel bookmarks",
		Args:  a.data.MaxArgs(1, "pixiv bookmark list [options] [USER_ID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runList(cmd, args, opts)
		},
	}
	a.bindListFlags(cmd, &opts)
	cmd.Flags().StringVarP(&opts.typ, "type", "t", opts.typ, "entity type: artwork or novel")
	cmd.Flags().StringVar(&opts.restrict, "restrict", opts.restrict, "bookmark visibility (public or private)")
	cmd.Flags().StringVar(&opts.tag, "tag", "", "filter by bookmark tag")
	a.data.BindTextValue(cmd, 0, 1, 0)
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

func (a command) newTags() *cobra.Command {
	opts := listOptions{typ: "artwork", restrict: string(pixiv.RestrictPublic)}
	cmd := &cobra.Command{
		Use:   "tags [USER_ID]",
		Short: "List artwork bookmark tags",
		Args:  a.data.MaxArgs(1, "pixiv bookmark tags [options] [USER_ID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runTags(cmd, args, opts)
		},
	}
	a.bindListFlags(cmd, &opts)
	cmd.Flags().StringVarP(&opts.typ, "type", "t", opts.typ, "entity type: artwork")
	cmd.Flags().StringVar(&opts.restrict, "restrict", opts.restrict, "bookmark visibility (public or private)")
	a.data.BindTextValue(cmd, 0, 1, 0)
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

func (a command) newDetail() *cobra.Command {
	options := deps.CommandOptions{}
	cmd := &cobra.Command{
		Use:   "detail ARTWORK_ID",
		Short: "Show the current user's bookmark detail",
		Args:  a.data.ExactArgs(1, "pixiv bookmark detail [options] ARTWORK_ID"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parse.PositiveInt64(args[0], "artwork_id")
			if err != nil {
				return err
			}
			request, err := a.data.Request(cmd, options)
			if err != nil {
				return err
			}
			jsonOut, err := a.data.JSONOut(a.data.JSONOverride(cmd, options))
			if err != nil {
				return err
			}
			result, err := deps.Read(a.data, cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) (pixiv.ArtworkBookmarkDetail, error) {
				return client.ArtworkBookmark(ctx, pixiv.ArtworkBookmarkRequest{ArtworkID: id})
			})
			if err != nil {
				return err
			}
			if jsonOut {
				return a.data.WriteJSON(pixiv.ToArtworkBookmarkDetailDTO(result))
			}
			if result.Restrict == "" {
				_, err = fmt.Fprintln(a.data.Output, "bookmarked: no")
				return err
			}
			if _, err := fmt.Fprintf(a.data.Output, "bookmarked: yes\nrestrict: %s\n", result.Restrict); err != nil {
				return err
			}
			_, err = fmt.Fprintf(a.data.Output, "tags: %s\n", strings.Join(result.Tags, ","))
			return err
		},
	}
	a.data.BindCommonFlags(cmd, &options)
	a.data.BindTextValue(cmd, 1, 1, 0)
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

func (a command) newAdd() *cobra.Command {
	opts := mutationOptions{restrict: string(pixiv.RestrictPublic)}
	cmd := &cobra.Command{
		Use:   "add [ILLUST_ID]",
		Short: "Bookmark an illustration",
		Args:  a.data.ActionInputArgs("pixiv bookmark add [options] [ILLUST_ID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := pipeline.RecordFailureStrategy(opts.onError); err != nil {
				return a.data.Usage(err)
			}
			invoke := a.actionInvoker(cmd, opts.CommandOptions, func(ctx context.Context, request deps.Request, id int64) error {
				return deps.Write(a.data, ctx, request, func(ctx context.Context, client *pixiv.Client) error {
					return client.AddBookmark(ctx, pixiv.AddBookmarkRequest{ArtworkID: id, Restrict: pixiv.Restrict(opts.restrict), Tags: opts.tags})
				})
			})
			if len(args) == 0 {
				return a.data.ConsumeActionRecords(cmd, "bookmark_add", opts.onError, visualRecordTypes, invoke)
			}
			id, err := parse.PositiveInt64(args[0], "illust_id")
			if err != nil {
				return a.data.Usage(err)
			}
			return invoke(cmd.Context(), id)
		},
	}
	a.data.BindActionFlags(cmd, &opts.ProxyOptions)
	cmd.Flags().StringVar(&opts.restrict, "restrict", opts.restrict, "bookmark visibility (public or private)")
	cmd.Flags().StringArrayVar(&opts.tags, "tag", nil, "bookmark tag; may be repeated")
	cmd.Flags().StringVar(&opts.onError, "on-error", "skip", "record failure strategy: skip or fail-fast")
	a.data.BindTextOrRecord(cmd, 0, 1, 0)
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

func (a command) newRemove() *cobra.Command {
	opts := mutationOptions{}
	cmd := &cobra.Command{
		Use:   "remove [ILLUST_ID]",
		Short: "Remove an illustration bookmark",
		Args:  a.data.ActionInputArgs("pixiv bookmark remove [options] [ILLUST_ID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := pipeline.RecordFailureStrategy(opts.onError); err != nil {
				return a.data.Usage(err)
			}
			invoke := a.actionInvoker(cmd, opts.CommandOptions, func(ctx context.Context, request deps.Request, id int64) error {
				return deps.Write(a.data, ctx, request, func(ctx context.Context, client *pixiv.Client) error {
					return client.RemoveBookmark(ctx, pixiv.RemoveBookmarkRequest{ArtworkID: id})
				})
			})
			if len(args) == 0 {
				return a.data.ConsumeActionRecords(cmd, "bookmark_remove", opts.onError, visualRecordTypes, invoke)
			}
			id, err := parse.PositiveInt64(args[0], "illust_id")
			if err != nil {
				return a.data.Usage(err)
			}
			return invoke(cmd.Context(), id)
		},
	}
	a.data.BindActionFlags(cmd, &opts.ProxyOptions)
	cmd.Flags().StringVar(&opts.onError, "on-error", "skip", "record failure strategy: skip or fail-fast")
	a.data.BindTextOrRecord(cmd, 0, 1, 0)
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

func (a command) bindListFlags(cmd *cobra.Command, opts *listOptions) {
	a.data.BindCommonFlags(cmd, &opts.CommandOptions)
	listing.BindNDJSONFlag(cmd, &opts.ndjson)
	listing.BindListFlags(cmd, &opts.limit, &opts.page)
}

func (a command) runList(cmd *cobra.Command, args []string, opts listOptions) error {
	switch opts.typ {
	case "artwork":
		return a.runArtworkList(cmd, args, opts)
	case "novel":
		return a.runNovelList(cmd, args, opts)
	default:
		return errors.New("bookmark list type must be one of artwork, novel")
	}
}

func (a command) runArtworkList(cmd *cobra.Command, args []string, opts listOptions) error {
	plan, err := listing.ParsePlan(cmd, opts.limit, opts.page)
	if err != nil {
		return err
	}
	var requestedUserID int64
	if len(args) == 1 {
		requestedUserID, err = parse.PositiveInt64(args[0], "user_id")
		if err != nil {
			return err
		}
	}
	request, err := a.data.Request(cmd, opts.CommandOptions)
	if err != nil {
		return err
	}
	if opts.ndjson && cmd.Flags().Changed("json") {
		return a.data.Usage(errors.New("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !opts.ndjson {
		jsonOut, err = a.data.JSONOut(a.data.JSONOverride(cmd, opts.CommandOptions))
		if err != nil {
			return err
		}
	}
	ndjson := a.data.ShouldAutoNDJSON(cmd, opts.ndjson, jsonOut)
	userID := requestedUserID
	fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		if userID == 0 {
			userID, err = deps.CurrentUserID(client)
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
		}
		result, err := client.UserArtworkBookmarks(ctx, pixiv.UserArtworkBookmarksRequest{UserID: userID, Restrict: pixiv.Restrict(opts.restrict), Tag: opts.tag, Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return listing.New(a.data.Output, a.data).RunPooledIllustListWithHeading(cmd.Context(), request, plan, jsonOut, ndjson, func() string {
		return fmt.Sprintf("bookmarks by %d", userID)
	}, fetch, func(items []pixiv.Artwork, start int) error {
		return printArtworks(a.data.Output, items, start)
	})
}

func (a command) runNovelList(cmd *cobra.Command, args []string, opts listOptions) error {
	plan, err := listing.ParsePlan(cmd, opts.limit, opts.page)
	if err != nil {
		return err
	}
	var requestedUserID int64
	if len(args) == 1 {
		requestedUserID, err = parse.PositiveInt64(args[0], "user_id")
		if err != nil {
			return err
		}
	}
	request, err := a.data.Request(cmd, opts.CommandOptions)
	if err != nil {
		return err
	}
	if opts.ndjson && cmd.Flags().Changed("json") {
		return a.data.Usage(errors.New("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !opts.ndjson {
		jsonOut, err = a.data.JSONOut(a.data.JSONOverride(cmd, opts.CommandOptions))
		if err != nil {
			return err
		}
	}
	ndjson := a.data.ShouldAutoNDJSON(cmd, opts.ndjson, jsonOut)
	userID := requestedUserID
	fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
		if userID == 0 {
			userID, err = deps.CurrentUserID(client)
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
		}
		result, err := client.UserNovelBookmarks(ctx, pixiv.UserNovelBookmarksRequest{UserID: userID, Restrict: pixiv.Restrict(opts.restrict), Tag: opts.tag, Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}
	return listing.New(a.data.Output, a.data).RunPooledNovelList(cmd.Context(), request, plan, jsonOut, ndjson, fmt.Sprintf("novel bookmarks by %d", userID), fetch, func(items []pixiv.Novel) error {
		return printNovels(a.data.Output, items)
	})
}

func (a command) runTags(cmd *cobra.Command, args []string, opts listOptions) error {
	if opts.typ != "artwork" {
		return errors.New("bookmark tags type must be artwork")
	}
	plan, err := listing.ParsePlan(cmd, opts.limit, opts.page)
	if err != nil {
		return err
	}
	var requestedUserID int64
	if len(args) == 1 {
		requestedUserID, err = parse.PositiveInt64(args[0], "user_id")
		if err != nil {
			return err
		}
	}
	request, err := a.data.Request(cmd, opts.CommandOptions)
	if err != nil {
		return err
	}
	if opts.ndjson && cmd.Flags().Changed("json") {
		return a.data.Usage(errors.New("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !opts.ndjson {
		jsonOut, err = a.data.JSONOut(a.data.JSONOverride(cmd, opts.CommandOptions))
		if err != nil {
			return err
		}
	}
	ndjson := a.data.ShouldAutoNDJSON(cmd, opts.ndjson, jsonOut)
	userID := requestedUserID
	return a.data.Pooled(cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) (bool, error) {
		fetch := func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.BookmarkTag, sdk.Cursor, error) {
			if userID == 0 {
				userID, err = deps.CurrentUserID(client)
				if err != nil {
					return nil, sdk.Cursor{}, err
				}
			}
			result, err := client.UserArtworkBookmarkTags(ctx, pixiv.UserArtworkBookmarkTagsRequest{UserID: userID, Restrict: pixiv.Restrict(opts.restrict), Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			return result.Items, result.Next, nil
		}
		var items []pixiv.BookmarkTag
		if err := listing.PageItems(ctx, plan, fetch, func(page []pixiv.BookmarkTag) error {
			items = append(items, page...)
			return nil
		}); err != nil {
			return false, err
		}
		if ndjson {
			for _, item := range items {
				if err := writeJSONLine(a.data.Output, pixiv.ToBookmarkTagDTO(item)); err != nil {
					return true, err
				}
			}
			return len(items) > 0, nil
		}
		if jsonOut {
			dtos := make([]pixiv.BookmarkTagDTO, 0, len(items))
			for _, item := range items {
				dtos = append(dtos, pixiv.ToBookmarkTagDTO(item))
			}
			if err := a.data.WriteJSON(struct {
				Tags []pixiv.BookmarkTagDTO `json:"bookmark_tags"`
			}{Tags: dtos}); err != nil {
				return true, err
			}
			return true, nil
		}
		for _, item := range items {
			if _, err := fmt.Fprintf(a.data.Output, "%s (%d)\n", item.Name, item.Count); err != nil {
				return true, err
			}
		}
		return true, nil
	})
}

func (a command) actionInvoker(cmd *cobra.Command, options deps.CommandOptions, invoke func(context.Context, deps.Request, int64) error) func(context.Context, int64) error {
	var request deps.Request
	initialized := false
	return func(ctx context.Context, id int64) error {
		if !initialized {
			resolved, err := a.data.Request(cmd, options)
			if err != nil {
				return err
			}
			request = resolved
			initialized = true
		}
		return invoke(ctx, request, id)
	}
}

func printArtworks(out io.Writer, items []pixiv.Artwork, offset int) error {
	_ = offset
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

func writeJSONLine(out io.Writer, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := out.Write(body); err != nil {
		return err
	}
	_, err = io.WriteString(out, "\n")
	return err
}
