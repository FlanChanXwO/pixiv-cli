// Package bookmark 提供 pixiv bookmark 命令组：作品与小说的收藏列表、标签、详情、添加与移除。
//
// 包的组成：[New] 组装命令树。
// 分页读取与输出共用 [github.com/FlanChanXwO/pixiv-cli/internal/session] 与 [github.com/FlanChanXwO/pixiv-cli/internal/record]。
package bookmark

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/target"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

var visualRecordTypes = map[string]struct{}{
	string(target.ResultKindArtwork): {},
	string(target.SubtypeIllust):     {},
	string(target.SubtypeManga):      {},
	string(target.SubtypeUgoira):     {},
}

// novelRecordTypes 与 frozen cli-migration-matrix 一致：bookmark add/remove 在
// --type novel 下只消费 novel namespace 的 record， artwork record 视为跨
// namespace 拒绝。
var novelRecordTypes = map[string]struct{}{
	string(target.ResultKindNovel): {},
}

type listOptions struct {
	environment.Options
	input    *environment.State
	ndjson   bool
	limit    int
	page     int
	typ      string
	restrict string
	tag      string
}

type detailOptions struct {
	environment.Options
	input *environment.State
	typ   string
}

type mutationOptions struct {
	environment.Options
	input    *environment.State
	typ      string
	restrict string
	tags     []string
	onError  string
}

type commandRunner struct {
	env    environment.Env
	runner session.Runner
	exec   session.Executor
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

// IsZero 报告 cursor 是否仍处于流的起点。
func (c bookmarkStreamCursor) IsZero() bool {
	return c.upstream.IsZero() && c.consumed == 0
}

// String 输出上游 cursor 与已消费数量的组合，供日志和错误文本使用。
func (c bookmarkStreamCursor) String() string {
	if c.IsZero() {
		return ""
	}
	return fmt.Sprintf("%s#%d", c.upstream.String(), c.consumed)
}

// New 构造 `pixiv bookmark` 命令组；作品与小说的收藏操作都经由同一个重放边界。
func New(env environment.Env, exec session.Executor) *cobra.Command {
	a := commandRunner{env: env, runner: session.NewRunner(env.Out, exec), exec: exec}
	cmd := &cobra.Command{Use: "bookmark", Short: "Manage artwork and novel bookmarks"}
	cmd.AddCommand(a.newList(), a.newTags(), a.newDetail(), a.newAdd(), a.newRemove())
	env.BindNoInput(cmd)
	return cmd
}

func (a commandRunner) newList() *cobra.Command {
	opts := listOptions{typ: string(target.ResultKindArtwork), restrict: string(pixiv.RestrictPublic)}
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
	opts.input = a.env.BindTextOrRecord(cmd, 0, 1, 0)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a commandRunner) newTags() *cobra.Command {
	opts := listOptions{typ: string(target.ResultKindArtwork), restrict: string(pixiv.RestrictPublic)}
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
	opts.input = a.env.BindTextOrRecord(cmd, 0, 1, 0)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a commandRunner) newDetail() *cobra.Command {
	opts := detailOptions{typ: string(target.ResultKindArtwork)}
	cmd := &cobra.Command{
		Use:   "detail ARTWORK_ID_OR_NOVEL_ID_OR_URL",
		Short: "Show the current user's bookmark detail",
		Args: func(cmd *cobra.Command, args []string) error {
			usage := "pixiv bookmark detail [options] ARTWORK_ID_OR_NOVEL_ID_OR_URL"
			if opts.input.Mode() == environment.RecordMode {
				if len(args) != 0 {
					return fmt.Errorf("usage: %s", usage)
				}
				return nil
			}
			return a.env.ExactArgs(1, usage)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			source, err := a.bookmarkDetailInput(cmd, args, opts)
			if err != nil {
				return err
			}
			resolved, err := target.Resolve(cmd.Context(), source, bookmarkDetailContract())
			if err != nil {
				return err
			}
			request, err := a.env.Request(cmd, opts.Options.ProxyOptions)
			if err != nil {
				return err
			}
			jsonOut, err := a.env.ResolveJSON(a.env.JSONOverride(cmd, opts.Options))
			if err != nil {
				return err
			}
			result, err := session.Read(a.runner, cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) (bookmarkDetailResult, error) {
				switch resolved.ResultKind {
				case target.ResultKindArtwork:
					value, err := client.ArtworkBookmark(ctx, pixiv.ArtworkBookmarkRequest{ArtworkID: resolved.ID})
					return bookmarkDetailResult{kind: string(target.ResultKindArtwork), artwork: value}, err
				case target.ResultKindNovel:
					value, err := client.NovelBookmark(ctx, pixiv.NovelBookmarkRequest{NovelID: resolved.ID})
					return bookmarkDetailResult{kind: string(target.ResultKindNovel), novel: value}, err
				default:
					return bookmarkDetailResult{}, errors.New("bookmark detail type must be artwork or novel")
				}
			})
			if err != nil {
				return err
			}
			if jsonOut {
				if result.kind == string(target.ResultKindNovel) {
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
	a.env.BindCommonFlags(cmd, &opts.Options)
	cmd.Flags().StringVarP(&opts.typ, "type", "t", opts.typ, "entity type: artwork or novel")
	opts.input = a.env.BindTextOrRecord(cmd, 0, 1, 0)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a commandRunner) bookmarkDetailInput(cmd *cobra.Command, args []string, opts detailOptions) (target.Input, error) {
	resolved := target.Input{Type: opts.typ}
	if opts.input.Mode() == environment.RecordMode {
		body, err := io.ReadAll(opts.input.Reader(a.env.In))
		if err != nil {
			return target.Input{}, fmt.Errorf("read bookmark detail record: %w", err)
		}
		source, err := record.ParseRecordJSON(body)
		if err != nil {
			return target.Input{}, err
		}
		resolved.Record = &source
		return resolved, nil
	}
	if len(args) != 1 {
		return target.Input{}, errors.New("bookmark detail requires one artwork or novel ID/URL")
	}
	resolved.Value = args[0]
	return resolved, nil
}

type bookmarkDetailResult struct {
	kind    string
	artwork pixiv.ArtworkBookmarkDetail
	novel   pixiv.NovelBookmarkDetail
}

func (r bookmarkDetailResult) restrictAndTags() (pixiv.Restrict, []string) {
	if r.kind == string(target.ResultKindNovel) {
		return r.novel.Restrict, r.novel.Tags
	}
	return r.artwork.Restrict, r.artwork.Tags
}

func bookmarkDetailContract() target.Contract {
	return target.Contract{
		Operation:   "bookmark detail",
		DefaultType: string(target.ResultKindArtwork),
		Types: []target.TypeSpec{
			{Name: string(target.ResultKindArtwork), ResultKind: target.ResultKindArtwork, BareReferenceKind: pixiv.ReferenceKindArtwork},
			{Name: string(target.ResultKindNovel), ResultKind: target.ResultKindNovel, BareReferenceKind: pixiv.ReferenceKindNovel},
		},
		URLKinds: map[pixiv.ReferenceKind]target.URLTypeRelation{
			pixiv.ReferenceKindArtwork: target.URLTypeMustMatchResult,
			pixiv.ReferenceKindNovel:   target.URLTypeMustMatchResult,
		},
	}
}

// bookmarkMutationType 校验 add/remove 的显式 entity type。frozen
// cli-migration-matrix 将 bookmark detail/add/remove 冻结为 artwork 或 novel；
// 不接受 all 或其他 namespace。错误措辞与 resolver 的 TypeSpec 校验保持一致。
func bookmarkMutationType(typ string) (string, error) {
	switch typ {
	case string(target.ResultKindArtwork), string(target.ResultKindNovel):
		return typ, nil
	default:
		return "", fmt.Errorf("type %q is not supported by this command", typ)
	}
}

func (a commandRunner) newAdd() *cobra.Command {
	opts := mutationOptions{typ: string(target.ResultKindArtwork), restrict: string(pixiv.RestrictPublic)}
	cmd := &cobra.Command{
		Use:   "add [ARTWORK_ID_OR_NOVEL_ID]",
		Short: "Bookmark an artwork or novel",
		Args:  a.env.ActionInputArgs("pixiv bookmark add [options] [ARTWORK_ID_OR_NOVEL_ID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRestrict(opts.restrict); err != nil {
				return err
			}
			typ, err := bookmarkMutationType(opts.typ)
			if err != nil {
				return a.env.WrapUsage(err)
			}
			if _, err := record.RecordFailureStrategy(opts.onError); err != nil {
				return a.env.WrapUsage(err)
			}
			invoke := a.actionInvoker(cmd, opts.Options, func(ctx context.Context, request session.ProxyRequest, id int64) error {
				return a.runner.Write(ctx, request, func(ctx context.Context, client *pixiv.Client) error {
					if typ == string(target.ResultKindNovel) {
						return client.AddNovelBookmark(ctx, pixiv.AddNovelBookmarkRequest{NovelID: id, Restrict: pixiv.Restrict(opts.restrict), Tags: opts.tags})
					}
					return client.AddArtworkBookmark(ctx, pixiv.AddArtworkBookmarkRequest{ArtworkID: id, Restrict: pixiv.Restrict(opts.restrict), Tags: opts.tags})
				})
			})
			if len(args) == 0 {
				recordTypes := visualRecordTypes
				if typ == string(target.ResultKindNovel) {
					recordTypes = novelRecordTypes
				}
				return a.env.ConsumeActionRecords(cmd, opts.input, "bookmark_add", opts.onError, recordTypes, invoke)
			}
			id, err := environment.PositiveInt64(args[0], "bookmark target ID")
			if err != nil {
				return a.env.WrapUsage(err)
			}
			return invoke(cmd.Context(), id)
		},
	}
	a.env.BindProxyFlags(cmd, &opts.ProxyOptions)
	cmd.Flags().StringVarP(&opts.typ, "type", "t", opts.typ, "entity type: artwork or novel")
	cmd.Flags().StringVar(&opts.restrict, "restrict", opts.restrict, "bookmark visibility (public or private)")
	cmd.Flags().StringArrayVar(&opts.tags, "tag", nil, "bookmark tag; may be repeated")
	cmd.Flags().StringVar(&opts.onError, "on-error", record.RecordFailureSkip, "record failure strategy: skip or fail-fast")
	opts.input = a.env.BindTextOrRecord(cmd, 0, 1, 0)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a commandRunner) newRemove() *cobra.Command {
	opts := mutationOptions{typ: string(target.ResultKindArtwork)}
	cmd := &cobra.Command{
		Use:   "remove [ARTWORK_ID_OR_NOVEL_ID]",
		Short: "Remove an artwork or novel bookmark",
		Args:  a.env.ActionInputArgs("pixiv bookmark remove [options] [ARTWORK_ID_OR_NOVEL_ID]"),
		RunE: func(cmd *cobra.Command, args []string) error {
			typ, err := bookmarkMutationType(opts.typ)
			if err != nil {
				return a.env.WrapUsage(err)
			}
			if _, err := record.RecordFailureStrategy(opts.onError); err != nil {
				return a.env.WrapUsage(err)
			}
			invoke := a.actionInvoker(cmd, opts.Options, func(ctx context.Context, request session.ProxyRequest, id int64) error {
				return a.runner.Write(ctx, request, func(ctx context.Context, client *pixiv.Client) error {
					if typ == string(target.ResultKindNovel) {
						return client.RemoveNovelBookmark(ctx, pixiv.RemoveNovelBookmarkRequest{NovelID: id})
					}
					return client.RemoveArtworkBookmark(ctx, pixiv.RemoveArtworkBookmarkRequest{ArtworkID: id})
				})
			})
			if len(args) == 0 {
				recordTypes := visualRecordTypes
				if typ == string(target.ResultKindNovel) {
					recordTypes = novelRecordTypes
				}
				return a.env.ConsumeActionRecords(cmd, opts.input, "bookmark_remove", opts.onError, recordTypes, invoke)
			}
			id, err := environment.PositiveInt64(args[0], "bookmark target ID")
			if err != nil {
				return a.env.WrapUsage(err)
			}
			return invoke(cmd.Context(), id)
		},
	}
	a.env.BindProxyFlags(cmd, &opts.ProxyOptions)
	cmd.Flags().StringVarP(&opts.typ, "type", "t", opts.typ, "entity type: artwork or novel")
	cmd.Flags().StringVar(&opts.onError, "on-error", record.RecordFailureSkip, "record failure strategy: skip or fail-fast")
	opts.input = a.env.BindTextOrRecord(cmd, 0, 1, 0)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a commandRunner) bindListFlags(cmd *cobra.Command, opts *listOptions) {
	a.env.BindCommonFlags(cmd, &opts.Options)
	environment.BindNDJSONFlag(cmd, &opts.ndjson)
	environment.BindListFlags(cmd, &opts.limit, &opts.page)
}

func (a commandRunner) runList(cmd *cobra.Command, args []string, opts listOptions) error {
	if err := validateRestrict(opts.restrict); err != nil {
		return err
	}
	switch opts.typ {
	case string(target.ResultKindArtwork):
		return a.runArtworkList(cmd, args, opts)
	case string(target.ResultKindNovel):
		return a.runNovelList(cmd, args, opts)
	case "all":
		return a.runAllList(cmd, args, opts)
	default:
		return errors.New("bookmark list type must be one of artwork, novel, all")
	}
}

func bookmarkContract(operation string) target.Contract {
	return target.Contract{
		Operation:   operation,
		DefaultType: string(target.ResultKindArtwork),
		Types: []target.TypeSpec{
			{Name: string(target.ResultKindArtwork), ResultKind: target.ResultKindArtwork, BareReferenceKind: pixiv.ReferenceKindUser},
			{Name: string(target.ResultKindNovel), ResultKind: target.ResultKindNovel, BareReferenceKind: pixiv.ReferenceKindUser},
			{Name: "all", All: true, BareReferenceKind: pixiv.ReferenceKindUser},
		},
		URLKinds: map[pixiv.ReferenceKind]target.URLTypeRelation{
			pixiv.ReferenceKindUser:          target.URLTypeIndependent,
			pixiv.ReferenceKindUserBookmarks: target.URLTypeMustMatchResult,
		},
	}
}

func bookmarkListContract() target.Contract {
	return bookmarkContract("bookmark list")
}

func bookmarkTagsContract() target.Contract {
	return bookmarkContract("bookmark tags")
}

func (a commandRunner) resolveBookmarkUserID(cmd *cobra.Command, args []string, state *environment.State, typ string, contract target.Contract) (int64, error) {
	resolved := target.Input{Type: typ}
	if state.Mode() == environment.RecordMode {
		body, err := io.ReadAll(state.Reader(a.env.In))
		if err != nil {
			return 0, fmt.Errorf("read bookmark target record: %w", err)
		}
		source, err := record.ParseRecordJSON(body)
		if err != nil {
			return 0, err
		}
		resolved.Record = &source
	} else {
		if len(args) != 1 {
			return 0, errors.New("bookmark target requires one user ID or URL")
		}
		resolved.Value = args[0]
	}
	result, err := target.Resolve(cmd.Context(), resolved, contract)
	if err != nil {
		return 0, err
	}
	if result.TargetKind != target.TargetKindUser {
		return 0, errors.New("bookmark target must identify a user")
	}
	return result.ID, nil
}

func (a commandRunner) actionInvoker(cmd *cobra.Command, options environment.Options, invoke func(context.Context, session.ProxyRequest, int64) error) func(context.Context, int64) error {
	var request session.ProxyRequest
	initialized := false
	return func(ctx context.Context, id int64) error {
		if !initialized {
			resolved, err := a.env.Request(cmd, options.ProxyOptions)
			if err != nil {
				return err
			}
			request = resolved
			initialized = true
		}
		return invoke(ctx, request, id)
	}
}

func validateRestrict(value string) error {
	switch pixiv.Restrict(value) {
	case "", pixiv.RestrictPublic, pixiv.RestrictPrivate:
		return nil
	default:
		return errors.New("restrict must be one of: public, private; see pixiv bookmark --help")
	}
}
