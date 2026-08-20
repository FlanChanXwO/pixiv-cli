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
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/parse"
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
	return cmd
}

type commentOut struct {
	Comments      []pixiv.CommentDTO             `json:"comments"`
	Total         *int64                         `json:"total,omitempty"`
	AccessControl *pixiv.CommentAccessControlDTO `json:"access_control,omitempty"`
}

func (a command) run(cmd *cobra.Command, arg string, opts options) error {
	if opts.typ != "artwork" && opts.typ != "novel" {
		return errors.New("type must be one of artwork, novel")
	}
	if !cmd.Flags().Changed("type") {
		return errors.New("--type is required for comment")
	}
	id, err := parse.PositiveInt64(arg, "id")
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
			switch opts.typ {
			case "artwork":
				page, pageErr = client.ArtworkComments(ctx, pixiv.ArtworkCommentsRequest{ArtworkID: id, Cursor: cursor})
			case "novel":
				page, pageErr = client.NovelComments(ctx, pixiv.NovelCommentsRequest{NovelID: id, Cursor: cursor})
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
		if _, err := fmt.Fprintf(a.data.Output, "%s comments for %d\n", opts.typ, id); err != nil {
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
