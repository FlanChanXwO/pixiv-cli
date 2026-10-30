// Package comment 提供 pixiv comment 命令：评论列表、发表、回复、删除和 stamp 列表。
//
// 包的组成：[New] 组装命令树。
// 目标解析共用 [github.com/FlanChanXwO/pixiv-cli/internal/pixiv/target]。
package comment

import (
	"context"
	"errors"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/display"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/target"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

type options struct {
	environment.Options
	ndjson bool
	limit  int
	page   int
	typ    string
}

type mutationOptions struct {
	environment.Options
	comment         string
	parentCommentID int64
	stampID         int64
	typ             string
}

type stampsOptions struct {
	environment.Options
	ndjson bool
}

type commandRunner struct {
	env    environment.Env
	runner session.Runner
	exec   session.Executor
}

// New 构造 `pixiv comment` 命令及其增删改子命令；ID 统一由请求解析器确定作品或小说。
func New(env environment.Env, exec session.Executor) *cobra.Command {
	a := commandRunner{env: env, runner: session.NewRunner(env.Out, exec), exec: exec}
	options := options{}
	cmd := &cobra.Command{
		Use:   "comment ID",
		Short: "List artwork or novel comments",
		Args:  env.ExactArgs(1, "pixiv comment [options] ID"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.run(cmd, args[0], options)
		},
	}
	env.BindCommonFlags(cmd, &options.Options)
	environment.BindNDJSONFlag(cmd, &options.ndjson)
	environment.BindListFlags(cmd, &options.limit, &options.page)
	cmd.Flags().StringVarP(&options.typ, "type", "t", "", "entity type: artwork or novel (required)")
	env.BindTextValue(cmd, 1, 1, 0)
	environment.Bind(cmd, environment.Normal())
	cmd.AddCommand(a.newCreate(), a.newDelete(), a.newReply(), a.newStamp(), a.newStamps())
	return cmd
}

func (a commandRunner) newCreate() *cobra.Command {
	opts := mutationOptions{}
	cmd := &cobra.Command{
		Use:   "create ID",
		Short: "Create an artwork or novel comment",
		Args:  a.env.ExactArgs(1, "pixiv comment create [options] ID"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runCreate(cmd, args[0], opts)
		},
	}
	a.bindMutationFlags(cmd, &opts, "comment body (required)")
	return cmd
}

func (a commandRunner) newDelete() *cobra.Command {
	opts := mutationOptions{}
	cmd := &cobra.Command{
		Use:   "delete COMMENT_ID",
		Short: "Delete an artwork or novel comment",
		Args:  a.env.ExactArgs(1, "pixiv comment delete [options] COMMENT_ID"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runDelete(cmd, args[0], opts)
		},
	}
	a.bindMutationFlags(cmd, &opts, "")
	return cmd
}

func (a commandRunner) newReply() *cobra.Command {
	opts := mutationOptions{}
	cmd := &cobra.Command{
		Use:   "reply ID",
		Short: "Reply to an artwork or novel comment",
		Args:  a.env.ExactArgs(1, "pixiv comment reply [options] ID"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runReply(cmd, args[0], opts)
		},
	}
	a.bindMutationFlags(cmd, &opts, "comment body (required)")
	cmd.Flags().Int64Var(&opts.parentCommentID, "parent-comment-id", 0, "parent comment ID (required)")
	return cmd
}

func (a commandRunner) newStamp() *cobra.Command {
	opts := mutationOptions{}
	cmd := &cobra.Command{
		Use:   "stamp ID",
		Short: "Create a stamp comment on an artwork or novel",
		Args:  a.env.ExactArgs(1, "pixiv comment stamp [options] ID"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runStamp(cmd, args[0], opts)
		},
	}
	a.bindMutationFlags(cmd, &opts, "optional comment body (empty for sticker-only)")
	cmd.Flags().Int64Var(&opts.stampID, "stamp-id", 0, "stamp ID (required)")
	return cmd
}

func (a commandRunner) newStamps() *cobra.Command {
	opts := stampsOptions{}
	cmd := &cobra.Command{
		Use:   "stamps",
		Short: "List available comment stamps",
		Args:  a.env.ExactArgs(0, "pixiv comment stamps [options]"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.runStamps(cmd, opts)
		},
	}
	a.env.BindCommonFlags(cmd, &opts.Options)
	environment.BindNDJSONFlag(cmd, &opts.ndjson)
	a.env.BindNoInput(cmd)
	environment.Bind(cmd, environment.Normal())
	return cmd
}

func (a commandRunner) bindMutationFlags(cmd *cobra.Command, opts *mutationOptions, commentUsage string) {
	a.env.BindCommonFlags(cmd, &opts.Options)
	cmd.Flags().StringVarP(&opts.typ, "type", "t", "", "entity type: artwork or novel (required)")
	if commentUsage != "" {
		cmd.Flags().StringVar(&opts.comment, "comment", "", commentUsage)
	}
	a.env.BindTextValue(cmd, 1, 1, 0)
	environment.Bind(cmd, environment.Normal())
}

func (a commandRunner) runCreate(cmd *cobra.Command, arg string, opts mutationOptions) error {
	if err := validateCommentType(cmd, opts.typ); err != nil {
		return err
	}
	if opts.comment == "" {
		return errors.New("--comment is required")
	}
	resolved, err := target.Resolve(cmd.Context(), target.Input{Value: arg, Type: opts.typ}, commentWorkContract("comment create"))
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
	var result pixiv.CommentMutationResult
	err = a.runner.Write(cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) error {
		switch resolved.TargetKind {
		case target.TargetKindArtwork:
			result, err = client.PostArtworkComment(ctx, pixiv.PostArtworkCommentRequest{ArtworkID: resolved.ID, Comment: opts.comment})
		case target.TargetKindNovel:
			result, err = client.PostNovelComment(ctx, pixiv.PostNovelCommentRequest{NovelID: resolved.ID, Comment: opts.comment})
		default:
			return errors.New("comment target must be artwork or novel")
		}
		return err
	})
	if err != nil {
		return err
	}
	return a.writeMutationResult(jsonOut, result)
}

func (a commandRunner) runDelete(cmd *cobra.Command, arg string, opts mutationOptions) error {
	if err := validateCommentType(cmd, opts.typ); err != nil {
		return err
	}
	resolved, err := target.Resolve(cmd.Context(), target.Input{Value: arg, Type: opts.typ}, commentDeleteContract())
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
	err = a.runner.Write(cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) error {
		switch opts.typ {
		case string(target.ResultKindArtwork):
			return client.DeleteArtworkComment(ctx, pixiv.DeleteArtworkCommentRequest{CommentID: resolved.ID})
		case string(target.ResultKindNovel):
			return client.DeleteNovelComment(ctx, pixiv.DeleteNovelCommentRequest{CommentID: resolved.ID})
		default:
			return errors.New("type must be one of artwork, novel")
		}
	})
	if err != nil {
		return err
	}
	return a.writeDeleteResult(jsonOut, resolved.ID)
}

func (a commandRunner) runReply(cmd *cobra.Command, arg string, opts mutationOptions) error {
	if err := validateCommentType(cmd, opts.typ); err != nil {
		return err
	}
	if opts.comment == "" {
		return errors.New("--comment is required")
	}
	if opts.parentCommentID <= 0 {
		return errors.New("--parent-comment-id must be positive")
	}
	resolved, err := target.Resolve(cmd.Context(), target.Input{Value: arg, Type: opts.typ}, commentWorkContract("comment reply"))
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
	var result pixiv.CommentMutationResult
	err = a.runner.Write(cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) error {
		switch resolved.TargetKind {
		case target.TargetKindArtwork:
			result, err = client.ReplyArtworkComment(ctx, pixiv.ReplyArtworkCommentRequest{
				ArtworkID:       resolved.ID,
				Comment:         opts.comment,
				ParentCommentID: opts.parentCommentID,
			})
		case target.TargetKindNovel:
			result, err = client.ReplyNovelComment(ctx, pixiv.ReplyNovelCommentRequest{
				NovelID:         resolved.ID,
				Comment:         opts.comment,
				ParentCommentID: opts.parentCommentID,
			})
		default:
			return errors.New("comment target must be artwork or novel")
		}
		return err
	})
	if err != nil {
		return err
	}
	return a.writeMutationResult(jsonOut, result)
}

func (a commandRunner) runStamp(cmd *cobra.Command, arg string, opts mutationOptions) error {
	if err := validateCommentType(cmd, opts.typ); err != nil {
		return err
	}
	// 只发 stamp 的评论会显式保留空的 --comment，SDK 据此区分纯贴纸和带文字评论。
	if opts.stampID <= 0 {
		return errors.New("--stamp-id must be positive")
	}
	resolved, err := target.Resolve(cmd.Context(), target.Input{Value: arg, Type: opts.typ}, commentWorkContract("comment stamp"))
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
	var result pixiv.CommentMutationResult
	err = a.runner.Write(cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) error {
		switch resolved.TargetKind {
		case target.TargetKindArtwork:
			result, err = client.StampArtworkComment(ctx, pixiv.StampArtworkCommentRequest{
				ArtworkID: resolved.ID,
				Comment:   opts.comment,
				StampID:   opts.stampID,
			})
		case target.TargetKindNovel:
			result, err = client.StampNovelComment(ctx, pixiv.StampNovelCommentRequest{
				NovelID: resolved.ID,
				Comment: opts.comment,
				StampID: opts.stampID,
			})
		default:
			return errors.New("comment target must be artwork or novel")
		}
		return err
	})
	if err != nil {
		return err
	}
	return a.writeMutationResult(jsonOut, result)
}

func (a commandRunner) runStamps(cmd *cobra.Command, opts stampsOptions) error {
	request, err := a.env.Request(cmd, opts.Options.ProxyOptions)
	if err != nil {
		return err
	}
	if opts.ndjson && cmd.Flags().Changed("json") {
		return a.env.WrapUsage(errors.New("--ndjson cannot be used with --json"))
	}
	jsonOut := false
	if !opts.ndjson {
		jsonOut, err = a.env.ResolveJSON(a.env.JSONOverride(cmd, opts.Options))
		if err != nil {
			return err
		}
	}
	ndjson := a.env.AutoNDJSON(cmd, opts.ndjson, jsonOut)
	stamps, err := session.Read(a.runner, cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) ([]pixiv.Stamp, error) {
		return client.Stamps(ctx, pixiv.StampsRequest{})
	})
	if err != nil {
		return err
	}
	if ndjson {
		for _, item := range stamps {
			if err := record.WriteJSONLine(a.env.Out, item); err != nil {
				return err
			}
		}
		return nil
	}
	if jsonOut {
		return a.env.PrintJSON(struct {
			Stamps []pixiv.Stamp `json:"stamps"`
		}{Stamps: stamps})
	}
	if len(stamps) == 0 {
		_, err := fmt.Fprintln(a.env.Err, "No results.")
		return err
	}
	for _, item := range stamps {
		if _, err := fmt.Fprintf(a.env.Out, "%d %s\n", item.ID, item.Image.Resource.Ref.String()); err != nil {
			return err
		}
	}
	return nil
}

func validateCommentType(cmd *cobra.Command, typ string) error {
	if !cmd.Flags().Changed("type") {
		return errors.New("--type is required for comment")
	}
	if typ != string(target.ResultKindArtwork) && typ != string(target.ResultKindNovel) {
		return errors.New("type must be one of artwork, novel")
	}
	return nil
}

func resolveCommentTarget(cmd *cobra.Command, arg, typ string, contract target.Contract) (target.Target, error) {
	if err := validateCommentType(cmd, typ); err != nil {
		return target.Target{}, err
	}
	return target.Resolve(cmd.Context(), target.Input{Value: arg, Type: typ}, contract)
}

func commentWorkContract(operation string) target.Contract {
	return target.Contract{
		Operation: operation,
		Types: []target.TypeSpec{
			{Name: string(target.ResultKindArtwork), ResultKind: target.ResultKindComment, BareReferenceKind: pixiv.ReferenceKindArtwork},
			{Name: string(target.ResultKindNovel), ResultKind: target.ResultKindComment, BareReferenceKind: pixiv.ReferenceKindNovel},
		},
	}
}

func commentDeleteContract() target.Contract {
	return target.Contract{
		Operation: "comment delete",
		Types: []target.TypeSpec{
			{Name: string(target.ResultKindArtwork), ResultKind: target.ResultKindComment, BareTargetKind: target.TargetKindComment},
			{Name: string(target.ResultKindNovel), ResultKind: target.ResultKindComment, BareTargetKind: target.TargetKindComment},
		},
	}
}

func (a commandRunner) writeMutationResult(jsonOut bool, result pixiv.CommentMutationResult) error {
	if jsonOut {
		return a.env.PrintJSON(commentMutationOut{CommentID: result.CommentID})
	}
	_, err := fmt.Fprintf(a.env.Out, "Posted comment %d\n", result.CommentID)
	return err
}

func (a commandRunner) writeDeleteResult(jsonOut bool, commentID int64) error {
	if jsonOut {
		return a.env.PrintJSON(commentDeleteOut{Deleted: true})
	}
	_, err := fmt.Fprintf(a.env.Out, "Deleted comment %d\n", commentID)
	return err
}

type commentOut struct {
	Comments      []pixiv.Comment             `json:"comments"`
	Total         *int64                      `json:"total,omitempty"`
	AccessControl *pixiv.CommentAccessControl `json:"access_control,omitempty"`
}

type commentMutationOut struct {
	CommentID int64 `json:"comment_id"`
}

type commentDeleteOut struct {
	Deleted bool `json:"deleted"`
}

func (a commandRunner) run(cmd *cobra.Command, arg string, opts options) error {
	resolved, err := resolveCommentTarget(cmd, arg, opts.typ, commentWorkContract("comment read"))
	if err != nil {
		return err
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

	return a.exec(cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) (bool, error) {
		var metadata commentPageMetadata
		var comments []pixiv.Comment
		fetch := func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.Comment, sdk.Cursor, error) {
			var page pixiv.CommentPage
			var pageErr error
			switch resolved.TargetKind {
			case target.TargetKindArtwork:
				page, pageErr = client.ArtworkComments(ctx, pixiv.ArtworkCommentsRequest{ArtworkID: resolved.ID, Cursor: cursor})
			case target.TargetKindNovel:
				page, pageErr = client.NovelComments(ctx, pixiv.NovelCommentsRequest{NovelID: resolved.ID, Cursor: cursor})
			default:
				return nil, sdk.Cursor{}, errors.New("comment target must be artwork or novel")
			}
			if pageErr != nil {
				return nil, sdk.Cursor{}, pageErr
			}
			metadata.set(page)
			return page.Page.Items, page.Page.Next, nil
		}
		if err := pagination.PageItems(ctx, plan, fetch, func(items []pixiv.Comment) error {
			comments = append(comments, items...)
			return nil
		}); err != nil {
			return false, err
		}
		if ndjson {
			for _, item := range comments {
				if err := record.WriteJSONLine(a.env.Out, item); err != nil {
					return true, err
				}
			}
			return len(comments) > 0, nil
		}
		if jsonOut {
			var accessControl *pixiv.CommentAccessControl
			if metadata.access != nil {
				dto := *metadata.access
				accessControl = &dto
			}
			if err := a.env.PrintJSON(commentOut{Comments: comments, Total: metadata.total, AccessControl: accessControl}); err != nil {
				return true, err
			}
			return true, nil
		}
		if len(comments) == 0 {
			_, err := fmt.Fprintln(a.env.Err, "No results.")
			return false, err
		}
		for _, item := range comments {
			if _, err := fmt.Fprintf(a.env.Out, "%d  %s: %s\n", item.ID, display.SafeLine(item.User.Name), display.SafeLine(item.Comment)); err != nil {
				return true, err
			}
		}
		return true, nil
	})
}

type commentPageMetadata struct {
	total  *int64
	access *pixiv.CommentAccessControl
}

func (m *commentPageMetadata) set(page pixiv.CommentPage) {
	if m.total == nil && page.Total != nil {
		value := *page.Total
		m.total = &value
	}
	if m.access == nil && page.AccessControl != nil {
		value := *page.AccessControl
		m.access = &value
	}
}
