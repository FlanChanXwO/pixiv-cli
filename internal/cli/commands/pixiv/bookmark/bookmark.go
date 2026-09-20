// Package bookmark owns Pixiv bookmark reads and mutations.
package bookmark

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	requirements "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/internal/listing"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/pipeline"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/pagination"
	record "github.com/FlanChanXwO/pixiv-cli/internal/shared/record"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/resolver"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/parse"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

var visualRecordTypes = map[string]struct{}{
	"artwork": {},
	"illust":  {},
	"manga":   {},
	"ugoira":  {},
}

// novelRecordTypes 与 frozen cli-migration-matrix 一致：bookmark add/remove 在
// --type novel 下只消费 novel namespace 的 record， artwork record 视为跨
// namespace 拒绝。
var novelRecordTypes = map[string]struct{}{
	"novel": {},
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
	typ      string
	restrict string
	tags     []string
	onError  string
}

type command struct {
	data deps.Data
}

type bookmarkListItem struct {
	kind    string
	artwork pixiv.Artwork
	novel   pixiv.Novel
}

type bookmarkTagItem struct {
	kind string
	tag  pixiv.BookmarkTag
}

type typedBookmarkTagDTO struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
	Type  string `json:"type"`
}

// bookmarkStreamCursor 只在一次 all 聚合执行中记录“上游输入 cursor + 已消费位置”。
// checkpoint 重新请求同一批并跳过已消费前缀，避免 logical limit 截断时丢失批内余项；
// 它不解析或持久化上游 cursor。
type bookmarkStreamCursor struct {
	upstream sdk.Cursor
	consumed int
}

func (c bookmarkStreamCursor) IsZero() bool {
	return c.upstream.IsZero() && c.consumed == 0
}

func (c bookmarkStreamCursor) String() string {
	if c.IsZero() {
		return ""
	}
	return fmt.Sprintf("%s#%d", c.upstream.String(), c.consumed)
}

// New builds the actual `pixiv bookmark` group command.
func New(data deps.Data) *cobra.Command {
	a := command{data: data}
	cmd := &cobra.Command{Use: "bookmark", Short: "Manage artwork and novel bookmarks"}
	cmd.AddCommand(a.newList(), a.newTags(), a.newDetail(), a.newAdd(), a.newRemove())
	data.BindNoInput(cmd)
	return cmd
}

func (a command) newList() *cobra.Command {
	opts := listOptions{typ: "artwork", restrict: string(pixiv.RestrictPublic)}
	cmd := &cobra.Command{
		Use:   "list [USER_ID_OR_URL]",
		Short: "List artwork or novel bookmarks",
		Args:  a.data.MaxArgs(1, "pixiv bookmark list [options] [USER_ID_OR_URL]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runList(cmd, args, opts)
		},
	}
	a.bindListFlags(cmd, &opts)
	cmd.Flags().StringVarP(&opts.typ, "type", "t", opts.typ, "entity type: artwork, novel, or all")
	cmd.Flags().StringVar(&opts.restrict, "restrict", opts.restrict, "bookmark visibility (public or private)")
	cmd.Flags().StringVar(&opts.tag, "tag", "", "filter by bookmark tag")
	a.data.BindTextOrRecord(cmd, 0, 1, 0)
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

func (a command) newTags() *cobra.Command {
	opts := listOptions{typ: "artwork", restrict: string(pixiv.RestrictPublic)}
	cmd := &cobra.Command{
		Use:   "tags [USER_ID_OR_URL]",
		Short: "List artwork or novel bookmark tags",
		Args:  a.data.MaxArgs(1, "pixiv bookmark tags [options] [USER_ID_OR_URL]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runTags(cmd, args, opts)
		},
	}
	a.bindListFlags(cmd, &opts)
	cmd.Flags().StringVarP(&opts.typ, "type", "t", opts.typ, "entity type: artwork, novel, or all")
	cmd.Flags().StringVar(&opts.restrict, "restrict", opts.restrict, "bookmark visibility (public or private)")
	a.data.BindTextOrRecord(cmd, 0, 1, 0)
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

func (a command) newDetail() *cobra.Command {
	options := deps.CommandOptions{}
	typ := "artwork"
	cmd := &cobra.Command{
		Use:   "detail ARTWORK_ID_OR_NOVEL_ID_OR_URL",
		Short: "Show the current user's bookmark detail",
		Args: func(cmd *cobra.Command, args []string) error {
			usage := "pixiv bookmark detail [options] ARTWORK_ID_OR_NOVEL_ID_OR_URL"
			if pipeline.ModeOf(cmd) == pipeline.RecordMode {
				if len(args) != 0 {
					return fmt.Errorf("usage: %s", usage)
				}
				return nil
			}
			return a.data.ExactArgs(1, usage)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			input, err := a.bookmarkDetailInput(cmd, args, typ)
			if err != nil {
				return err
			}
			target, err := resolver.Resolve(cmd.Context(), input, bookmarkDetailContract())
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
			result, err := deps.Read(a.data, cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) (bookmarkDetailResult, error) {
				switch target.ResultKind {
				case resolver.ResultKindArtwork:
					value, err := client.ArtworkBookmark(ctx, pixiv.ArtworkBookmarkRequest{ArtworkID: target.ID})
					return bookmarkDetailResult{kind: "artwork", artwork: value}, err
				case resolver.ResultKindNovel:
					value, err := client.NovelBookmark(ctx, pixiv.NovelBookmarkRequest{NovelID: target.ID})
					return bookmarkDetailResult{kind: "novel", novel: value}, err
				default:
					return bookmarkDetailResult{}, errors.New("bookmark detail type must be artwork or novel")
				}
			})
			if err != nil {
				return err
			}
			if jsonOut {
				if result.kind == "novel" {
					return a.data.WriteJSON(pixiv.ToNovelBookmarkDetailDTO(result.novel))
				}
				return a.data.WriteJSON(pixiv.ToArtworkBookmarkDetailDTO(result.artwork))
			}
			restrict, tags := result.restrictAndTags()
			if restrict == "" {
				_, err = fmt.Fprintln(a.data.Output, "bookmarked: no")
				return err
			}
			if _, err := fmt.Fprintf(a.data.Output, "bookmarked: yes\nrestrict: %s\n", restrict); err != nil {
				return err
			}
			_, err = fmt.Fprintf(a.data.Output, "tags: %s\n", strings.Join(tags, ","))
			return err
		},
	}
	a.data.BindCommonFlags(cmd, &options)
	cmd.Flags().StringVarP(&typ, "type", "t", typ, "entity type: artwork or novel")
	a.data.BindTextOrRecord(cmd, 0, 1, 0)
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

func (a command) bookmarkDetailInput(cmd *cobra.Command, args []string, typ string) (resolver.Input, error) {
	input := resolver.Input{Type: typ}
	if pipeline.ModeOf(cmd) == pipeline.RecordMode {
		body, err := io.ReadAll(pipeline.Reader(cmd, a.data.Input))
		if err != nil {
			return resolver.Input{}, fmt.Errorf("read bookmark detail record: %w", err)
		}
		source, err := record.ParseRecordJSON(body)
		if err != nil {
			return resolver.Input{}, err
		}
		input.Record = &source
		return input, nil
	}
	if len(args) != 1 {
		return resolver.Input{}, errors.New("bookmark detail requires one artwork or novel ID/URL")
	}
	input.Value = args[0]
	return input, nil
}

type bookmarkDetailResult struct {
	kind    string
	artwork pixiv.ArtworkBookmarkDetail
	novel   pixiv.NovelBookmarkDetail
}

func (r bookmarkDetailResult) restrictAndTags() (pixiv.Restrict, []string) {
	if r.kind == "novel" {
		return r.novel.Restrict, r.novel.Tags
	}
	return r.artwork.Restrict, r.artwork.Tags
}

func bookmarkDetailContract() resolver.Contract {
	return resolver.Contract{
		Operation:   "bookmark detail",
		DefaultType: "artwork",
		Types: []resolver.TypeSpec{
			{Name: "artwork", ResultKind: resolver.ResultKindArtwork, BareReferenceKind: pixiv.ReferenceKindArtwork},
			{Name: "novel", ResultKind: resolver.ResultKindNovel, BareReferenceKind: pixiv.ReferenceKindNovel},
		},
		URLKinds: map[pixiv.ReferenceKind]resolver.URLTypeRelation{
			pixiv.ReferenceKindArtwork: resolver.URLTypeMustMatchResult,
			pixiv.ReferenceKindNovel:   resolver.URLTypeMustMatchResult,
		},
	}
}

// bookmarkMutationType 校验 add/remove 的显式 entity type。frozen
// cli-migration-matrix 将 bookmark detail/add/remove 冻结为 artwork 或 novel；
// 不接受 all 或其他 namespace。错误措辞与 resolver 的 TypeSpec 校验保持一致。
func bookmarkMutationType(typ string) (string, error) {
	switch typ {
	case "artwork", "novel":
		return typ, nil
	default:
		return "", fmt.Errorf("type %q is not supported by this command", typ)
	}
}

func (a command) newAdd() *cobra.Command {
	opts := mutationOptions{typ: "artwork", restrict: string(pixiv.RestrictPublic)}
	cmd := &cobra.Command{
		Use:   "add [ARTWORK_ID_OR_NOVEL_ID]",
		Short: "Bookmark an artwork or novel",
		Args:  a.data.ActionInputArgs("pixiv bookmark add [options] [ARTWORK_ID_OR_NOVEL_ID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			typ, err := bookmarkMutationType(opts.typ)
			if err != nil {
				return a.data.Usage(err)
			}
			if _, err := pipeline.RecordFailureStrategy(opts.onError); err != nil {
				return a.data.Usage(err)
			}
			invoke := a.actionInvoker(cmd, opts.CommandOptions, func(ctx context.Context, request deps.Request, id int64) error {
				return deps.Write(a.data, ctx, request, func(ctx context.Context, client *pixiv.Client) error {
					if typ == "novel" {
						return client.AddNovelBookmark(ctx, pixiv.AddNovelBookmarkRequest{NovelID: id, Restrict: pixiv.Restrict(opts.restrict), Tags: opts.tags})
					}
					return client.AddBookmark(ctx, pixiv.AddBookmarkRequest{ArtworkID: id, Restrict: pixiv.Restrict(opts.restrict), Tags: opts.tags})
				})
			})
			if len(args) == 0 {
				recordTypes := visualRecordTypes
				if typ == "novel" {
					recordTypes = novelRecordTypes
				}
				return a.data.ConsumeActionRecords(cmd, "bookmark_add", opts.onError, recordTypes, invoke)
			}
			id, err := parse.PositiveInt64(args[0], "bookmark target ID")
			if err != nil {
				return a.data.Usage(err)
			}
			return invoke(cmd.Context(), id)
		},
	}
	a.data.BindActionFlags(cmd, &opts.ProxyOptions)
	cmd.Flags().StringVarP(&opts.typ, "type", "t", opts.typ, "entity type: artwork or novel")
	cmd.Flags().StringVar(&opts.restrict, "restrict", opts.restrict, "bookmark visibility (public or private)")
	cmd.Flags().StringArrayVar(&opts.tags, "tag", nil, "bookmark tag; may be repeated")
	cmd.Flags().StringVar(&opts.onError, "on-error", "skip", "record failure strategy: skip or fail-fast")
	a.data.BindTextOrRecord(cmd, 0, 1, 0)
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

func (a command) newRemove() *cobra.Command {
	opts := mutationOptions{typ: "artwork"}
	cmd := &cobra.Command{
		Use:   "remove [ARTWORK_ID_OR_NOVEL_ID]",
		Short: "Remove an artwork or novel bookmark",
		Args:  a.data.ActionInputArgs("pixiv bookmark remove [options] [ARTWORK_ID_OR_NOVEL_ID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			typ, err := bookmarkMutationType(opts.typ)
			if err != nil {
				return a.data.Usage(err)
			}
			if _, err := pipeline.RecordFailureStrategy(opts.onError); err != nil {
				return a.data.Usage(err)
			}
			invoke := a.actionInvoker(cmd, opts.CommandOptions, func(ctx context.Context, request deps.Request, id int64) error {
				return deps.Write(a.data, ctx, request, func(ctx context.Context, client *pixiv.Client) error {
					if typ == "novel" {
						return client.RemoveNovelBookmark(ctx, pixiv.RemoveNovelBookmarkRequest{NovelID: id})
					}
					return client.RemoveBookmark(ctx, pixiv.RemoveBookmarkRequest{ArtworkID: id})
				})
			})
			if len(args) == 0 {
				recordTypes := visualRecordTypes
				if typ == "novel" {
					recordTypes = novelRecordTypes
				}
				return a.data.ConsumeActionRecords(cmd, "bookmark_remove", opts.onError, recordTypes, invoke)
			}
			id, err := parse.PositiveInt64(args[0], "bookmark target ID")
			if err != nil {
				return a.data.Usage(err)
			}
			return invoke(cmd.Context(), id)
		},
	}
	a.data.BindActionFlags(cmd, &opts.ProxyOptions)
	cmd.Flags().StringVarP(&opts.typ, "type", "t", opts.typ, "entity type: artwork or novel")
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
	case "all":
		return a.runAllList(cmd, args, opts)
	default:
		return errors.New("bookmark list type must be one of artwork, novel, all")
	}
}

func (a command) runAllList(cmd *cobra.Command, args []string, opts listOptions) error {
	plan, err := listing.ParsePlan(cmd, opts.limit, opts.page)
	if err != nil {
		return err
	}
	var userID int64
	if len(args) == 1 || pipeline.ModeOf(cmd) == pipeline.RecordMode {
		userID, err = a.resolveBookmarkUserID(cmd, args, opts.typ, bookmarkListContract())
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

	return a.data.Pooled(cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) (bool, error) {
		attemptUserID := userID
		if attemptUserID == 0 {
			resolvedUserID, err := deps.CurrentUserID(client)
			if err != nil {
				return false, err
			}
			attemptUserID = resolvedUserID
		}
		streams := []pagination.Stream[bookmarkListItem, bookmarkStreamCursor]{
			bookmarkArtworkStream(client, attemptUserID, opts),
			bookmarkNovelStream(client, attemptUserID, opts),
		}
		items, _, _, err := pagination.CollectStreams(ctx, plan.PagePlan(), streams)
		if err != nil {
			return false, err
		}

		var staged bytes.Buffer
		if ndjson {
			for _, item := range items {
				value, err := item.record()
				if err != nil {
					return false, err
				}
				if err := writeJSONLine(&staged, value); err != nil {
					return false, err
				}
			}
		} else if jsonOut {
			records := make([]record.Record, 0, len(items))
			for _, item := range items {
				value, err := item.record()
				if err != nil {
					return false, err
				}
				records = append(records, value)
			}
			if err := writeJSONValue(&staged, struct {
				Records []record.Record `json:"records"`
			}{Records: records}); err != nil {
				return false, err
			}
		} else {
			for _, item := range items {
				var err error
				switch item.kind {
				case "artwork":
					err = printArtworks(&staged, []pixiv.Artwork{item.artwork}, 0)
				case "novel":
					err = printNovels(&staged, []pixiv.Novel{item.novel})
				}
				if err != nil {
					return false, err
				}
			}
		}
		_, err = io.Copy(a.data.Output, &staged)
		return true, err
	})
}

func bookmarkArtworkStream(client *pixiv.Client, userID int64, opts listOptions) pagination.Stream[bookmarkListItem, bookmarkStreamCursor] {
	return pagination.Stream[bookmarkListItem, bookmarkStreamCursor]{
		Fetch: func(ctx context.Context, cursor bookmarkStreamCursor) ([]bookmarkListItem, bookmarkStreamCursor, error) {
			result, err := client.UserArtworkBookmarks(ctx, pixiv.UserArtworkBookmarksRequest{
				UserID: userID, Restrict: pixiv.Restrict(opts.restrict), Tag: opts.tag, Cursor: cursor.upstream,
			})
			if err != nil {
				return nil, bookmarkStreamCursor{}, err
			}
			if cursor.consumed > len(result.Items) {
				return nil, bookmarkStreamCursor{}, errors.New("bookmark artwork checkpoint exceeds upstream batch")
			}
			items := make([]bookmarkListItem, 0, len(result.Items)-cursor.consumed)
			for _, item := range result.Items[cursor.consumed:] {
				items = append(items, bookmarkListItem{kind: "artwork", artwork: item})
			}
			return items, bookmarkStreamCursor{upstream: result.Next}, nil
		},
		Checkpoint: bookmarkCheckpoint,
	}
}

func bookmarkNovelStream(client *pixiv.Client, userID int64, opts listOptions) pagination.Stream[bookmarkListItem, bookmarkStreamCursor] {
	return pagination.Stream[bookmarkListItem, bookmarkStreamCursor]{
		Fetch: func(ctx context.Context, cursor bookmarkStreamCursor) ([]bookmarkListItem, bookmarkStreamCursor, error) {
			result, err := client.UserNovelBookmarks(ctx, pixiv.UserNovelBookmarksRequest{
				UserID: userID, Restrict: pixiv.Restrict(opts.restrict), Tag: opts.tag, Cursor: cursor.upstream,
			})
			if err != nil {
				return nil, bookmarkStreamCursor{}, err
			}
			if cursor.consumed > len(result.Items) {
				return nil, bookmarkStreamCursor{}, errors.New("bookmark novel checkpoint exceeds upstream batch")
			}
			items := make([]bookmarkListItem, 0, len(result.Items)-cursor.consumed)
			for _, item := range result.Items[cursor.consumed:] {
				items = append(items, bookmarkListItem{kind: "novel", novel: item})
			}
			return items, bookmarkStreamCursor{upstream: result.Next}, nil
		},
		Checkpoint: bookmarkCheckpoint,
	}
}

func bookmarkCheckpoint(cursor bookmarkStreamCursor, position int) (bookmarkStreamCursor, error) {
	if position <= 0 {
		return bookmarkStreamCursor{}, errors.New("bookmark checkpoint position must be positive")
	}
	return bookmarkStreamCursor{upstream: cursor.upstream, consumed: cursor.consumed + position}, nil
}

func (item bookmarkListItem) record() (record.Record, error) {
	switch item.kind {
	case "artwork":
		return record.RecordFromArtworkDTO(pixiv.ToArtworkDTO(item.artwork))
	case "novel":
		return record.RecordFromNovelDTO(pixiv.ToNovelDTO(item.novel))
	default:
		return record.Record{}, fmt.Errorf("unsupported bookmark item kind %q", item.kind)
	}
}

func (a command) runArtworkList(cmd *cobra.Command, args []string, opts listOptions) error {
	plan, err := listing.ParsePlan(cmd, opts.limit, opts.page)
	if err != nil {
		return err
	}
	var requestedUserID int64
	if len(args) == 1 || pipeline.ModeOf(cmd) == pipeline.RecordMode {
		requestedUserID, err = a.resolveBookmarkUserID(cmd, args, opts.typ, bookmarkListContract())
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
	return a.runner().RunPooledIllustListWithHeading(cmd.Context(), listing.Request(request), plan, jsonOut, ndjson, func() string {
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
	if len(args) == 1 || pipeline.ModeOf(cmd) == pipeline.RecordMode {
		requestedUserID, err = a.resolveBookmarkUserID(cmd, args, opts.typ, bookmarkListContract())
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
	return a.runner().RunPooledNovelList(cmd.Context(), listing.Request(request), plan, jsonOut, ndjson, fmt.Sprintf("novel bookmarks by %d", userID), fetch, func(items []pixiv.Novel) error {
		return printNovels(a.data.Output, items)
	})
}

func (a command) runner() listing.Runner {
	return listing.New(a.data.Output, func(ctx context.Context, request listing.Request, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
		return a.data.Pooled(ctx, deps.Request(request), attempt)
	})
}

func bookmarkContract(operation string) resolver.Contract {
	return resolver.Contract{
		Operation:   operation,
		DefaultType: "artwork",
		Types: []resolver.TypeSpec{
			{Name: "artwork", ResultKind: resolver.ResultKindArtwork, BareReferenceKind: pixiv.ReferenceKindUser},
			{Name: "novel", ResultKind: resolver.ResultKindNovel, BareReferenceKind: pixiv.ReferenceKindUser},
			{Name: "all", All: true, BareReferenceKind: pixiv.ReferenceKindUser},
		},
		URLKinds: map[pixiv.ReferenceKind]resolver.URLTypeRelation{
			pixiv.ReferenceKindUser:          resolver.URLTypeIndependent,
			pixiv.ReferenceKindUserBookmarks: resolver.URLTypeMustMatchResult,
		},
	}
}

func bookmarkListContract() resolver.Contract {
	return bookmarkContract("bookmark list")
}

func bookmarkTagsContract() resolver.Contract {
	return bookmarkContract("bookmark tags")
}

func (a command) resolveBookmarkUserID(cmd *cobra.Command, args []string, typ string, contract resolver.Contract) (int64, error) {
	input := resolver.Input{Type: typ}
	if pipeline.ModeOf(cmd) == pipeline.RecordMode {
		body, err := io.ReadAll(pipeline.Reader(cmd, a.data.Input))
		if err != nil {
			return 0, fmt.Errorf("read bookmark target record: %w", err)
		}
		source, err := record.ParseRecordJSON(body)
		if err != nil {
			return 0, err
		}
		input.Record = &source
	} else {
		if len(args) != 1 {
			return 0, errors.New("bookmark target requires one user ID or URL")
		}
		input.Value = args[0]
	}
	target, err := resolver.Resolve(cmd.Context(), input, contract)
	if err != nil {
		return 0, err
	}
	if target.TargetKind != resolver.TargetKindUser {
		return 0, errors.New("bookmark target must identify a user")
	}
	return target.ID, nil
}

func (a command) runTags(cmd *cobra.Command, args []string, opts listOptions) error {
	if opts.typ == "all" {
		return a.runAllTags(cmd, args, opts)
	}
	if opts.typ != "artwork" && opts.typ != "novel" {
		return errors.New("bookmark tags type must be one of artwork, novel, all")
	}
	plan, err := listing.ParsePlan(cmd, opts.limit, opts.page)
	if err != nil {
		return err
	}
	var requestedUserID int64
	if len(args) == 1 || pipeline.ModeOf(cmd) == pipeline.RecordMode {
		requestedUserID, err = a.resolveBookmarkUserID(cmd, args, opts.typ, bookmarkTagsContract())
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
			var result sdk.Page[pixiv.BookmarkTag]
			if opts.typ == "novel" {
				result, err = client.UserNovelBookmarkTags(ctx, pixiv.UserNovelBookmarkTagsRequest{UserID: userID, Restrict: pixiv.Restrict(opts.restrict), Cursor: cursor})
			} else {
				result, err = client.UserArtworkBookmarkTags(ctx, pixiv.UserArtworkBookmarkTagsRequest{UserID: userID, Restrict: pixiv.Restrict(opts.restrict), Cursor: cursor})
			}
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

func (a command) runAllTags(cmd *cobra.Command, args []string, opts listOptions) error {
	plan, err := listing.ParsePlan(cmd, opts.limit, opts.page)
	if err != nil {
		return err
	}
	var userID int64
	if len(args) == 1 || pipeline.ModeOf(cmd) == pipeline.RecordMode {
		userID, err = a.resolveBookmarkUserID(cmd, args, opts.typ, bookmarkTagsContract())
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

	return a.data.Pooled(cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) (bool, error) {
		attemptUserID := userID
		if attemptUserID == 0 {
			resolvedUserID, err := deps.CurrentUserID(client)
			if err != nil {
				return false, err
			}
			attemptUserID = resolvedUserID
		}
		streams := []pagination.Stream[bookmarkTagItem, bookmarkStreamCursor]{
			bookmarkArtworkTagStream(client, attemptUserID, opts),
			bookmarkNovelTagStream(client, attemptUserID, opts),
		}
		items, _, _, err := pagination.CollectStreams(ctx, plan.PagePlan(), streams)
		if err != nil {
			return false, err
		}

		var staged bytes.Buffer
		if ndjson {
			for _, item := range items {
				if err := writeJSONLine(&staged, item.dto()); err != nil {
					return false, err
				}
			}
		} else if jsonOut {
			dtos := make([]typedBookmarkTagDTO, 0, len(items))
			for _, item := range items {
				dtos = append(dtos, item.dto())
			}
			if err := writeJSONValue(&staged, struct {
				Tags []typedBookmarkTagDTO `json:"bookmark_tags"`
			}{Tags: dtos}); err != nil {
				return false, err
			}
		} else {
			for _, item := range items {
				if _, err := fmt.Fprintf(&staged, "%s: %s (%d)\n", item.kind, item.tag.Name, item.tag.Count); err != nil {
					return false, err
				}
			}
		}
		_, err = io.Copy(a.data.Output, &staged)
		return true, err
	})
}

func bookmarkArtworkTagStream(client *pixiv.Client, userID int64, opts listOptions) pagination.Stream[bookmarkTagItem, bookmarkStreamCursor] {
	return pagination.Stream[bookmarkTagItem, bookmarkStreamCursor]{
		Fetch: func(ctx context.Context, cursor bookmarkStreamCursor) ([]bookmarkTagItem, bookmarkStreamCursor, error) {
			result, err := client.UserArtworkBookmarkTags(ctx, pixiv.UserArtworkBookmarkTagsRequest{
				UserID: userID, Restrict: pixiv.Restrict(opts.restrict), Cursor: cursor.upstream,
			})
			if err != nil {
				return nil, bookmarkStreamCursor{}, err
			}
			if cursor.consumed > len(result.Items) {
				return nil, bookmarkStreamCursor{}, errors.New("bookmark artwork tag checkpoint exceeds upstream batch")
			}
			items := make([]bookmarkTagItem, 0, len(result.Items)-cursor.consumed)
			for _, item := range result.Items[cursor.consumed:] {
				items = append(items, bookmarkTagItem{kind: "artwork", tag: item})
			}
			return items, bookmarkStreamCursor{upstream: result.Next}, nil
		},
		Checkpoint: bookmarkCheckpoint,
	}
}

func bookmarkNovelTagStream(client *pixiv.Client, userID int64, opts listOptions) pagination.Stream[bookmarkTagItem, bookmarkStreamCursor] {
	return pagination.Stream[bookmarkTagItem, bookmarkStreamCursor]{
		Fetch: func(ctx context.Context, cursor bookmarkStreamCursor) ([]bookmarkTagItem, bookmarkStreamCursor, error) {
			result, err := client.UserNovelBookmarkTags(ctx, pixiv.UserNovelBookmarkTagsRequest{
				UserID: userID, Restrict: pixiv.Restrict(opts.restrict), Cursor: cursor.upstream,
			})
			if err != nil {
				return nil, bookmarkStreamCursor{}, err
			}
			if cursor.consumed > len(result.Items) {
				return nil, bookmarkStreamCursor{}, errors.New("bookmark novel tag checkpoint exceeds upstream batch")
			}
			items := make([]bookmarkTagItem, 0, len(result.Items)-cursor.consumed)
			for _, item := range result.Items[cursor.consumed:] {
				items = append(items, bookmarkTagItem{kind: "novel", tag: item})
			}
			return items, bookmarkStreamCursor{upstream: result.Next}, nil
		},
		Checkpoint: bookmarkCheckpoint,
	}
}

func (item bookmarkTagItem) dto() typedBookmarkTagDTO {
	return typedBookmarkTagDTO{Name: item.tag.Name, Count: item.tag.Count, Type: item.kind}
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

func writeJSONValue(out io.Writer, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	_, err = out.Write(body)
	return err
}
