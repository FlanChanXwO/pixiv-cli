// Package bookmark owns Pixiv bookmark reads and mutations.
package bookmark

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	inputcmd "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/input"
	pixivcmd "github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/pixivcmd"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/resolver"
	record "github.com/FlanChanXwO/pixiv-cli/internal/record"
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
	command.Options
	ndjson   bool
	limit    int
	page     int
	typ      string
	restrict string
	tag      string
}

type mutationOptions struct {
	command.Options
	typ      string
	restrict string
	tags     []string
	onError  string
}

type commandRunner struct {
	env    command.Env
	runner pixivcmd.Runner
	exec   pixivcmd.Executor
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
func New(env command.Env, exec pixivcmd.Executor) *cobra.Command {
	a := commandRunner{env: env, runner: pixivcmd.New(env.Out, exec), exec: exec}
	cmd := &cobra.Command{Use: "bookmark", Short: "Manage artwork and novel bookmarks"}
	cmd.AddCommand(a.newList(), a.newTags(), a.newDetail(), a.newAdd(), a.newRemove())
	env.BindNoInput(cmd)
	return cmd
}

func (a commandRunner) newList() *cobra.Command {
	opts := listOptions{typ: "artwork", restrict: string(pixiv.RestrictPublic)}
	cmd := &cobra.Command{
		Use:   "list [USER_ID_OR_URL]",
		Short: "List artwork or novel bookmarks",
		Args:  a.env.MaxArgs(1, "pixiv bookmark list [options] [USER_ID_OR_URL]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runList(cmd, args, opts)
		},
	}
	a.bindListFlags(cmd, &opts)
	cmd.Flags().StringVarP(&opts.typ, "type", "t", opts.typ, "entity type: artwork, novel, or all")
	cmd.Flags().StringVar(&opts.restrict, "restrict", opts.restrict, "bookmark visibility (public or private)")
	cmd.Flags().StringVar(&opts.tag, "tag", "", "filter by bookmark tag")
	a.env.BindTextOrRecord(cmd, 0, 1, 0)
	command.Bind(cmd, command.PixivData())
	return cmd
}

func (a commandRunner) newTags() *cobra.Command {
	opts := listOptions{typ: "artwork", restrict: string(pixiv.RestrictPublic)}
	cmd := &cobra.Command{
		Use:   "tags [USER_ID_OR_URL]",
		Short: "List artwork or novel bookmark tags",
		Args:  a.env.MaxArgs(1, "pixiv bookmark tags [options] [USER_ID_OR_URL]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runTags(cmd, args, opts)
		},
	}
	a.bindListFlags(cmd, &opts)
	cmd.Flags().StringVarP(&opts.typ, "type", "t", opts.typ, "entity type: artwork, novel, or all")
	cmd.Flags().StringVar(&opts.restrict, "restrict", opts.restrict, "bookmark visibility (public or private)")
	a.env.BindTextOrRecord(cmd, 0, 1, 0)
	command.Bind(cmd, command.PixivData())
	return cmd
}

func (a commandRunner) newDetail() *cobra.Command {
	options := command.Options{}
	typ := "artwork"
	cmd := &cobra.Command{
		Use:   "detail ARTWORK_ID_OR_NOVEL_ID_OR_URL",
		Short: "Show the current user's bookmark detail",
		Args: func(cmd *cobra.Command, args []string) error {
			usage := "pixiv bookmark detail [options] ARTWORK_ID_OR_NOVEL_ID_OR_URL"
			if inputcmd.ModeOf(cmd) == inputcmd.RecordMode {
				if len(args) != 0 {
					return fmt.Errorf("usage: %s", usage)
				}
				return nil
			}
			return a.env.ExactArgs(1, usage)(cmd, args)
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
			request, err := pixivcmd.BuildRequest(cmd, options)
			if err != nil {
				return err
			}
			jsonOut, err := a.env.JSONOut(a.env.JSONOverride(cmd, options))
			if err != nil {
				return err
			}
			result, err := pixivcmd.Read(a.runner, cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) (bookmarkDetailResult, error) {
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
					return a.env.PrintJSON(result.novel)
				}
				return a.env.PrintJSON(result.artwork)
			}
			restrict, tags := result.restrictAndTags()
			if restrict == "" {
				_, err = fmt.Fprintln(a.env.Out, "bookmarked: no")
				return err
			}
			if _, err := fmt.Fprintf(a.env.Out, "bookmarked: yes\nrestrict: %s\n", restrict); err != nil {
				return err
			}
			_, err = fmt.Fprintf(a.env.Out, "tags: %s\n", strings.Join(tags, ","))
			return err
		},
	}
	a.env.BindCommonFlags(cmd, &options)
	cmd.Flags().StringVarP(&typ, "type", "t", typ, "entity type: artwork or novel")
	a.env.BindTextOrRecord(cmd, 0, 1, 0)
	command.Bind(cmd, command.PixivData())
	return cmd
}

func (a commandRunner) bookmarkDetailInput(cmd *cobra.Command, args []string, typ string) (resolver.Input, error) {
	input := resolver.Input{Type: typ}
	if inputcmd.ModeOf(cmd) == inputcmd.RecordMode {
		body, err := io.ReadAll(inputcmd.Reader(cmd, a.env.In))
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

func (a commandRunner) newAdd() *cobra.Command {
	opts := mutationOptions{typ: "artwork", restrict: string(pixiv.RestrictPublic)}
	cmd := &cobra.Command{
		Use:   "add [ARTWORK_ID_OR_NOVEL_ID]",
		Short: "Bookmark an artwork or novel",
		Args:  a.env.ActionInputArgs("pixiv bookmark add [options] [ARTWORK_ID_OR_NOVEL_ID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			typ, err := bookmarkMutationType(opts.typ)
			if err != nil {
				return a.env.UsageError(err)
			}
			if _, err := inputcmd.RecordFailureStrategy(opts.onError); err != nil {
				return a.env.UsageError(err)
			}
			invoke := a.actionInvoker(cmd, opts.Options, func(ctx context.Context, request pixivcmd.Request, id int64) error {
				return a.runner.Write(ctx, request, func(ctx context.Context, client *pixiv.Client) error {
					if typ == "novel" {
						return client.AddNovelBookmark(ctx, pixiv.AddNovelBookmarkRequest{NovelID: id, Restrict: pixiv.Restrict(opts.restrict), Tags: opts.tags})
					}
					return client.AddArtworkBookmark(ctx, pixiv.AddArtworkBookmarkRequest{ArtworkID: id, Restrict: pixiv.Restrict(opts.restrict), Tags: opts.tags})
				})
			})
			if len(args) == 0 {
				recordTypes := visualRecordTypes
				if typ == "novel" {
					recordTypes = novelRecordTypes
				}
				return a.env.ConsumeActionRecords(cmd, "bookmark_add", opts.onError, recordTypes, invoke)
			}
			id, err := inputcmd.PositiveInt64(args[0], "bookmark target ID")
			if err != nil {
				return a.env.UsageError(err)
			}
			return invoke(cmd.Context(), id)
		},
	}
	a.env.BindProxyFlags(cmd, &opts.ProxyOptions)
	cmd.Flags().StringVarP(&opts.typ, "type", "t", opts.typ, "entity type: artwork or novel")
	cmd.Flags().StringVar(&opts.restrict, "restrict", opts.restrict, "bookmark visibility (public or private)")
	cmd.Flags().StringArrayVar(&opts.tags, "tag", nil, "bookmark tag; may be repeated")
	cmd.Flags().StringVar(&opts.onError, "on-error", "skip", "record failure strategy: skip or fail-fast")
	a.env.BindTextOrRecord(cmd, 0, 1, 0)
	command.Bind(cmd, command.PixivData())
	return cmd
}

func (a commandRunner) newRemove() *cobra.Command {
	opts := mutationOptions{typ: "artwork"}
	cmd := &cobra.Command{
		Use:   "remove [ARTWORK_ID_OR_NOVEL_ID]",
		Short: "Remove an artwork or novel bookmark",
		Args:  a.env.ActionInputArgs("pixiv bookmark remove [options] [ARTWORK_ID_OR_NOVEL_ID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			typ, err := bookmarkMutationType(opts.typ)
			if err != nil {
				return a.env.UsageError(err)
			}
			if _, err := inputcmd.RecordFailureStrategy(opts.onError); err != nil {
				return a.env.UsageError(err)
			}
			invoke := a.actionInvoker(cmd, opts.Options, func(ctx context.Context, request pixivcmd.Request, id int64) error {
				return a.runner.Write(ctx, request, func(ctx context.Context, client *pixiv.Client) error {
					if typ == "novel" {
						return client.RemoveNovelBookmark(ctx, pixiv.RemoveNovelBookmarkRequest{NovelID: id})
					}
					return client.RemoveArtworkBookmark(ctx, pixiv.RemoveArtworkBookmarkRequest{ArtworkID: id})
				})
			})
			if len(args) == 0 {
				recordTypes := visualRecordTypes
				if typ == "novel" {
					recordTypes = novelRecordTypes
				}
				return a.env.ConsumeActionRecords(cmd, "bookmark_remove", opts.onError, recordTypes, invoke)
			}
			id, err := inputcmd.PositiveInt64(args[0], "bookmark target ID")
			if err != nil {
				return a.env.UsageError(err)
			}
			return invoke(cmd.Context(), id)
		},
	}
	a.env.BindProxyFlags(cmd, &opts.ProxyOptions)
	cmd.Flags().StringVarP(&opts.typ, "type", "t", opts.typ, "entity type: artwork or novel")
	cmd.Flags().StringVar(&opts.onError, "on-error", "skip", "record failure strategy: skip or fail-fast")
	a.env.BindTextOrRecord(cmd, 0, 1, 0)
	command.Bind(cmd, command.PixivData())
	return cmd
}

func (a commandRunner) bindListFlags(cmd *cobra.Command, opts *listOptions) {
	a.env.BindCommonFlags(cmd, &opts.Options)
	pixivcmd.BindNDJSONFlag(cmd, &opts.ndjson)
	pixivcmd.BindListFlags(cmd, &opts.limit, &opts.page)
}

func (a commandRunner) runList(cmd *cobra.Command, args []string, opts listOptions) error {
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

func (a commandRunner) resolveBookmarkUserID(cmd *cobra.Command, args []string, typ string, contract resolver.Contract) (int64, error) {
	input := resolver.Input{Type: typ}
	if inputcmd.ModeOf(cmd) == inputcmd.RecordMode {
		body, err := io.ReadAll(inputcmd.Reader(cmd, a.env.In))
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

func (a commandRunner) actionInvoker(cmd *cobra.Command, options command.Options, invoke func(context.Context, pixivcmd.Request, int64) error) func(context.Context, int64) error {
	var request pixivcmd.Request
	initialized := false
	return func(ctx context.Context, id int64) error {
		if !initialized {
			resolved, err := pixivcmd.BuildRequest(cmd, options)
			if err != nil {
				return err
			}
			request = resolved
			initialized = true
		}
		return invoke(ctx, request, id)
	}
}
