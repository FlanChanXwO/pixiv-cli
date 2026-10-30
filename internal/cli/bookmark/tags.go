package bookmark

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

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

func (a commandRunner) runTags(cmd *cobra.Command, args []string, opts listOptions) error {
	if err := validateRestrict(opts.restrict); err != nil {
		return err
	}
	if opts.typ == "all" {
		return a.runAllTags(cmd, args, opts)
	}
	if opts.typ != string(target.NamespaceArtwork) && opts.typ != string(target.NamespaceNovel) {
		return errors.New("bookmark tags type must be one of artwork, novel, all")
	}
	plan, err := environment.ParsePlan(cmd, opts.limit, opts.page)
	if err != nil {
		return err
	}
	var requestedUserID int64
	if len(args) == 1 || opts.input.Mode() == environment.RecordMode {
		requestedUserID, err = a.resolveBookmarkUserID(cmd, args, opts.input, opts.typ, bookmarkTagsContract())
		if err != nil {
			return err
		}
	}
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
	userID := requestedUserID
	return a.exec(cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) (bool, error) {
		fetch := func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.BookmarkTag, sdk.Cursor, error) {
			if userID == 0 {
				userID, err = session.CurrentUserID(client)
				if err != nil {
					return nil, sdk.Cursor{}, err
				}
			}
			var result sdk.Page[pixiv.BookmarkTag]
			if opts.typ == string(target.NamespaceNovel) {
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
		page, err := pagination.TraversePages(ctx, plan, fetch, func(page []pixiv.BookmarkTag) error {
			items = append(items, page...)
			return nil
		})
		if err != nil {
			return false, err
		}
		if ndjson {
			for _, item := range items {
				if err := record.WriteJSONLine(a.env.Out, item); err != nil {
					return true, err
				}
			}
			return len(items) > 0, nil
		}
		if jsonOut {
			if err := record.WriteJSONValue(a.env.Out, record.NewListOutput(items, plan, page)); err != nil {
				return true, err
			}
			return true, nil
		}
		if len(items) == 0 {
			_, err := fmt.Fprintln(a.env.Err, "No results.")
			return false, err
		}
		for _, item := range items {
			if _, err := fmt.Fprintf(a.env.Out, "%s (%d)\n", display.SafeLine(item.Name), item.Count); err != nil {
				return true, err
			}
		}
		return true, nil
	})
}

func (a commandRunner) runAllTags(cmd *cobra.Command, args []string, opts listOptions) error {
	plan, err := environment.ParsePlan(cmd, opts.limit, opts.page)
	if err != nil {
		return err
	}
	var userID int64
	if len(args) == 1 || opts.input.Mode() == environment.RecordMode {
		userID, err = a.resolveBookmarkUserID(cmd, args, opts.input, opts.typ, bookmarkTagsContract())
		if err != nil {
			return err
		}
	}
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

	return a.exec(cmd.Context(), request, func(ctx context.Context, client *pixiv.Client) (bool, error) {
		attemptUserID := userID
		if attemptUserID == 0 {
			resolvedUserID, err := session.CurrentUserID(client)
			if err != nil {
				return false, err
			}
			attemptUserID = resolvedUserID
		}
		streams := []pagination.Stream[bookmarkTagItem, bookmarkStreamCursor]{
			bookmarkArtworkTagStream(client, attemptUserID, opts),
			bookmarkNovelTagStream(client, attemptUserID, opts),
		}
		items, _, page, err := pagination.CollectStreams(ctx, plan, streams)
		if err != nil {
			return false, err
		}

		var staged bytes.Buffer
		if ndjson {
			for _, item := range items {
				if err := record.WriteJSONLine(&staged, item.dto()); err != nil {
					return false, err
				}
			}
		} else if jsonOut {
			dtos := make([]typedBookmarkTagDTO, 0, len(items))
			for _, item := range items {
				dtos = append(dtos, item.dto())
			}
			if err := record.WriteJSONValue(&staged, record.NewListOutput(dtos, plan, page)); err != nil {
				return false, err
			}
		} else {
			if len(items) == 0 {
				_, err := fmt.Fprintln(a.env.Err, "No results.")
				return false, err
			}
			for _, item := range items {
				if _, err := fmt.Fprintf(&staged, "%s (%d)  %s\n", display.SafeLine(item.tag.Name), item.tag.Count, item.kind); err != nil {
					return false, err
				}
			}
		}
		_, err = io.Copy(a.env.Out, &staged)
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
				items = append(items, bookmarkTagItem{kind: string(target.NamespaceArtwork), tag: item})
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
				items = append(items, bookmarkTagItem{kind: string(target.NamespaceNovel), tag: item})
			}
			return items, bookmarkStreamCursor{upstream: result.Next}, nil
		},
		Checkpoint: bookmarkCheckpoint,
	}
}

func (item bookmarkTagItem) dto() typedBookmarkTagDTO {
	return typedBookmarkTagDTO{Name: item.tag.Name, Count: item.tag.Count, Type: item.kind}
}
