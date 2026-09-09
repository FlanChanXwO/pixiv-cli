// Package comment owns the Pixiv comment listing command and its presenters.
package comment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	requirements "github.com/FlanChanXwO/pixiv-cli/internal/cli/commands"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/internal/listing"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/resolver"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/spf13/cobra"
)

type options struct {
	deps.CommandOptions
	ndjson bool
	limit  int
	page   int
	typ    string
}

type mutationOptions struct {
	deps.CommandOptions
	comment         string
	parentCommentID int64
	stampID         int64
	typ             string
}

type stampsOptions struct {
	deps.CommandOptions
	ndjson bool
}

type command struct {
	data deps.Data
}

// New builds the actual `pixiv comment` command.
func New(data deps.Data) *cobra.Command {
	a := command{data: data}
	options := options{}
	cmd := &cobra.Command{
		Use:   "comment ID",
		Short: "List artwork or novel comments",
		Args:  data.ExactArgs(1, "pixiv comment [options] ID"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.run(cmd, args[0], options)
		},
	}
	data.BindCommonFlags(cmd, &options.CommandOptions)
	listing.BindNDJSONFlag(cmd, &options.ndjson)
	listing.BindListFlags(cmd, &options.limit, &options.page)
	cmd.Flags().StringVarP(&options.typ, "type", "t", "", "entity type: artwork or novel (required)")
	data.BindTextValue(cmd, 1, 1, 0)
	requirements.Bind(cmd, requirements.PixivData())
	cmd.AddCommand(a.newCreate(), a.newDelete(), a.newReply(), a.newStamp(), a.newStamps())
	return cmd
}

func (a command) newCreate() *cobra.Command {
	opts := mutationOptions{}
	cmd := &cobra.Command{
		Use:   "create ID",
		Short: "Create an artwork or novel comment",
		Args:  a.data.ExactArgs(1, "pixiv comment create [options] ID"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runCreate(cmd, args[0], opts)
		},
	}
	a.bindMutationFlags(cmd, &opts, "comment body (required)")
	return cmd
}

func (a command) newDelete() *cobra.Command {
	opts := mutationOptions{}
	cmd := &cobra.Command{
		Use:   "delete COMMENT_ID",
		Short: "Delete an artwork or novel comment",
		Args:  a.data.ExactArgs(1, "pixiv comment delete [options] COMMENT_ID"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runDelete(cmd, args[0], opts)
		},
	}
	a.bindMutationFlags(cmd, &opts, "")
	return cmd
}

func (a command) newReply() *cobra.Command {
	opts := mutationOptions{}
	cmd := &cobra.Command{
		Use:   "reply ID",
		Short: "Reply to an artwork or novel comment",
		Args:  a.data.ExactArgs(1, "pixiv comment reply [options] ID"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runReply(cmd, args[0], opts)
		},
	}
	a.bindMutationFlags(cmd, &opts, "comment body (required)")
	cmd.Flags().Int64Var(&opts.parentCommentID, "parent-comment-id", 0, "parent comment ID (required)")
	return cmd
}

func (a command) newStamp() *cobra.Command {
	opts := mutationOptions{}
	cmd := &cobra.Command{
		Use:   "stamp ID",
		Short: "Create a stamp comment on an artwork or novel",
		Args:  a.data.ExactArgs(1, "pixiv comment stamp [options] ID"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runStamp(cmd, args[0], opts)
		},
	}
	a.bindMutationFlags(cmd, &opts, "comment body (required)")
	cmd.Flags().Int64Var(&opts.stampID, "stamp-id", 0, "stamp ID (required)")
	return cmd
}

func (a command) newStamps() *cobra.Command {
	opts := stampsOptions{}
	cmd := &cobra.Command{
		Use:   "stamps",
		Short: "List available comment stamps",
		Args:  a.data.ExactArgs(0, "pixiv comment stamps [options]"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.runStamps(cmd, opts)
		},
	}
	a.data.BindCommonFlags(cmd, &opts.CommandOptions)
	listing.BindNDJSONFlag(cmd, &opts.ndjson)
	a.data.BindNoInput(cmd)
	requirements.Bind(cmd, requirements.PixivData())
	return cmd
}

func (a command) bindMutationFlags(cmd *cobra.Command, opts *mutationOptions, commentUsage string) {
	a.data.BindCommonFlags(cmd, &opts.CommandOptions)
	cmd.Flags().StringVarP(&opts.typ, "type", "t", "", "entity type: artwork or novel (required)")
	if commentUsage != "" {
		cmd.Flags().StringVar(&opts.comment, "comment", "", commentUsage)
	}
	a.data.BindTextValue(cmd, 1, 1, 0)
	requirements.Bind(cmd, requirements.PixivData())
}

func (a command) runCreate(cmd *cobra.Command, arg string, opts mutationOptions) error {
	if err := validateCommentType(cmd, opts.typ); err != nil {
		return err
	}
	if opts.comment == "" {
		return errors.New("--comment is required")
	}
	target, err := resolver.Resolve(cmd.Context(), resolver.Input{Value: arg, Type: opts.typ}, commentWorkContract("comment create"))
	if err != nil {
		return err
	}
	request, err := a.data.Request(cmd, opts.CommandOptions)
	if err != nil {
		return err
	}
	jsonOut, err := a.data.JSONOut(a.data.JSONOverride(cmd, opts.CommandOptions))
	if err != nil {
		return err
	}
	var result pixiv.CommentMutationResult
	err = deps.Write(a.data, cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) error {
		switch target.TargetKind {
		case resolver.TargetKindArtwork:
			result, err = client.PostArtworkComment(ctx, pixiv.PostArtworkCommentRequest{ArtworkID: target.ID, Comment: opts.comment})
		case resolver.TargetKindNovel:
			result, err = client.PostNovelComment(ctx, pixiv.PostNovelCommentRequest{NovelID: target.ID, Comment: opts.comment})
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

func (a command) runDelete(cmd *cobra.Command, arg string, opts mutationOptions) error {
	if err := validateCommentType(cmd, opts.typ); err != nil {
		return err
	}
	target, err := resolver.Resolve(cmd.Context(), resolver.Input{Value: arg, Type: opts.typ}, commentDeleteContract())
	if err != nil {
		return err
	}
	request, err := a.data.Request(cmd, opts.CommandOptions)
	if err != nil {
		return err
	}
	jsonOut, err := a.data.JSONOut(a.data.JSONOverride(cmd, opts.CommandOptions))
	if err != nil {
		return err
	}
	err = deps.Write(a.data, cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) error {
		switch opts.typ {
		case "artwork":
			return client.DeleteArtworkComment(ctx, pixiv.DeleteArtworkCommentRequest{CommentID: target.ID})
		case "novel":
			return client.DeleteNovelComment(ctx, pixiv.DeleteNovelCommentRequest{CommentID: target.ID})
		default:
			return errors.New("type must be one of artwork, novel")
		}
	})
	if err != nil {
		return err
	}
	return a.writeDeleteResult(jsonOut)
}

func (a command) runReply(cmd *cobra.Command, arg string, opts mutationOptions) error {
	if err := validateCommentType(cmd, opts.typ); err != nil {
		return err
	}
	if opts.comment == "" {
		return errors.New("--comment is required")
	}
	if opts.parentCommentID <= 0 {
		return errors.New("--parent-comment-id must be positive")
	}
	target, err := resolver.Resolve(cmd.Context(), resolver.Input{Value: arg, Type: opts.typ}, commentWorkContract("comment reply"))
	if err != nil {
		return err
	}
	request, err := a.data.Request(cmd, opts.CommandOptions)
	if err != nil {
		return err
	}
	jsonOut, err := a.data.JSONOut(a.data.JSONOverride(cmd, opts.CommandOptions))
	if err != nil {
		return err
	}
	var result pixiv.CommentMutationResult
	err = deps.Write(a.data, cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) error {
		switch target.TargetKind {
		case resolver.TargetKindArtwork:
			result, err = client.ReplyArtworkComment(ctx, pixiv.ReplyArtworkCommentRequest{
				ArtworkID:       target.ID,
				Comment:         opts.comment,
				ParentCommentID: opts.parentCommentID,
			})
		case resolver.TargetKindNovel:
			result, err = client.ReplyNovelComment(ctx, pixiv.ReplyNovelCommentRequest{
				NovelID:         target.ID,
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

func (a command) runStamp(cmd *cobra.Command, arg string, opts mutationOptions) error {
	if err := validateCommentType(cmd, opts.typ); err != nil {
		return err
	}
	if opts.comment == "" {
		return errors.New("--comment is required")
	}
	if opts.stampID <= 0 {
		return errors.New("--stamp-id must be positive")
	}
	target, err := resolver.Resolve(cmd.Context(), resolver.Input{Value: arg, Type: opts.typ}, commentWorkContract("comment stamp"))
	if err != nil {
		return err
	}
	request, err := a.data.Request(cmd, opts.CommandOptions)
	if err != nil {
		return err
	}
	jsonOut, err := a.data.JSONOut(a.data.JSONOverride(cmd, opts.CommandOptions))
	if err != nil {
		return err
	}
	var result pixiv.CommentMutationResult
	err = deps.Write(a.data, cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) error {
		switch target.TargetKind {
		case resolver.TargetKindArtwork:
			result, err = client.StampArtworkComment(ctx, pixiv.StampArtworkCommentRequest{
				ArtworkID: target.ID,
				Comment:   opts.comment,
				StampID:   opts.stampID,
			})
		case resolver.TargetKindNovel:
			result, err = client.StampNovelComment(ctx, pixiv.StampNovelCommentRequest{
				NovelID: target.ID,
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

func (a command) runStamps(cmd *cobra.Command, opts stampsOptions) error {
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
	stamps, err := deps.Read(a.data, cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) ([]pixiv.Stamp, error) {
		return client.Stamps(ctx, pixiv.StampsRequest{})
	})
	if err != nil {
		return err
	}
	if ndjson {
		for _, item := range stamps {
			if err := writeJSONLine(a.data.Output, pixiv.ToStampDTO(item)); err != nil {
				return err
			}
		}
		return nil
	}
	if jsonOut {
		items := make([]pixiv.StampDTO, 0, len(stamps))
		for _, item := range stamps {
			items = append(items, pixiv.ToStampDTO(item))
		}
		return a.data.WriteJSON(struct {
			Stamps []pixiv.StampDTO `json:"stamps"`
		}{Stamps: items})
	}
	for _, item := range stamps {
		if _, err := fmt.Fprintf(a.data.Output, "%d %s\n", item.ID, item.Image.Resource.Ref.String()); err != nil {
			return err
		}
	}
	return nil
}

func validateCommentType(cmd *cobra.Command, typ string) error {
	if !cmd.Flags().Changed("type") {
		return errors.New("--type is required for comment")
	}
	if typ != "artwork" && typ != "novel" {
		return errors.New("type must be one of artwork, novel")
	}
	return nil
}

func resolveCommentTarget(cmd *cobra.Command, arg, typ string, contract resolver.Contract) (resolver.Target, error) {
	if err := validateCommentType(cmd, typ); err != nil {
		return resolver.Target{}, err
	}
	return resolver.Resolve(cmd.Context(), resolver.Input{Value: arg, Type: typ}, contract)
}

func commentWorkContract(operation string) resolver.Contract {
	return resolver.Contract{
		Operation: operation,
		Types: []resolver.TypeSpec{
			{Name: "artwork", ResultKind: resolver.ResultKindComment, BareReferenceKind: pixiv.ReferenceKindArtwork},
			{Name: "novel", ResultKind: resolver.ResultKindComment, BareReferenceKind: pixiv.ReferenceKindNovel},
		},
	}
}

func commentDeleteContract() resolver.Contract {
	return resolver.Contract{
		Operation: "comment delete",
		Types: []resolver.TypeSpec{
			{Name: "artwork", ResultKind: resolver.ResultKindComment, BareTargetKind: resolver.TargetKindComment},
			{Name: "novel", ResultKind: resolver.ResultKindComment, BareTargetKind: resolver.TargetKindComment},
		},
	}
}

func (a command) writeMutationResult(jsonOut bool, result pixiv.CommentMutationResult) error {
	if jsonOut {
		return a.data.WriteJSON(commentMutationOut{CommentID: result.CommentID})
	}
	_, err := fmt.Fprintf(a.data.Output, "comment id: %d\n", result.CommentID)
	return err
}

func (a command) writeDeleteResult(jsonOut bool) error {
	if jsonOut {
		return a.data.WriteJSON(commentDeleteOut{Deleted: true})
	}
	_, err := io.WriteString(a.data.Output, "comment deleted\n")
	return err
}

type commentOut struct {
	Comments      []pixiv.CommentDTO             `json:"comments"`
	Total         *int64                         `json:"total,omitempty"`
	AccessControl *pixiv.CommentAccessControlDTO `json:"access_control,omitempty"`
}

type commentMutationOut struct {
	CommentID int64 `json:"comment_id"`
}

type commentDeleteOut struct {
	Deleted bool `json:"deleted"`
}

func (a command) run(cmd *cobra.Command, arg string, opts options) error {
	target, err := resolveCommentTarget(cmd, arg, opts.typ, commentWorkContract("comment read"))
	if err != nil {
		return err
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

	return a.data.Pooled(cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) (bool, error) {
		var metadata commentPageMetadata
		var comments []pixiv.Comment
		fetch := func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.Comment, sdk.Cursor, error) {
			var page pixiv.CommentPage
			var pageErr error
			switch target.TargetKind {
			case resolver.TargetKindArtwork:
				page, pageErr = client.ArtworkComments(ctx, pixiv.ArtworkCommentsRequest{ArtworkID: target.ID, Cursor: cursor})
			case resolver.TargetKindNovel:
				page, pageErr = client.NovelComments(ctx, pixiv.NovelCommentsRequest{NovelID: target.ID, Cursor: cursor})
			default:
				return nil, sdk.Cursor{}, errors.New("comment target must be artwork or novel")
			}
			if pageErr != nil {
				return nil, sdk.Cursor{}, pageErr
			}
			metadata.set(page)
			return page.Page.Items, page.Page.Next, nil
		}
		if err := listing.PageItems(ctx, plan, fetch, func(items []pixiv.Comment) error {
			comments = append(comments, items...)
			return nil
		}); err != nil {
			return false, err
		}
		if ndjson {
			for _, item := range comments {
				if err := writeJSONLine(a.data.Output, pixiv.ToCommentDTO(item)); err != nil {
					return true, err
				}
			}
			return len(comments) > 0, nil
		}
		if jsonOut {
			commentDTOs := make([]pixiv.CommentDTO, 0, len(comments))
			for _, item := range comments {
				commentDTOs = append(commentDTOs, pixiv.ToCommentDTO(item))
			}
			var accessControl *pixiv.CommentAccessControlDTO
			if metadata.access != nil {
				dto := pixiv.ToCommentAccessControlDTO(*metadata.access)
				accessControl = &dto
			}
			if err := a.data.WriteJSON(commentOut{Comments: commentDTOs, Total: metadata.total, AccessControl: accessControl}); err != nil {
				return true, err
			}
			return true, nil
		}
		if _, err := fmt.Fprintf(a.data.Output, "%s comments for %d\n", opts.typ, target.ID); err != nil {
			return true, err
		}
		for _, item := range comments {
			if _, err := fmt.Fprintf(a.data.Output, "%d %s: %s\n", item.ID, item.User.Name, item.Comment); err != nil {
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
