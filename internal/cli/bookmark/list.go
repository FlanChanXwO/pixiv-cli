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

func (a commandRunner) runAllList(cmd *cobra.Command, args []string, opts listOptions) error {
	plan, err := environment.ParsePlan(cmd, opts.limit, opts.page)
	if err != nil {
		return err
	}
	var userID int64
	if len(args) == 1 || opts.input.Mode() == environment.RecordMode {
		userID, err = a.resolveBookmarkUserID(cmd, args, opts.input, opts.typ, bookmarkListContract())
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
		streams := []pagination.Stream[bookmarkListItem, bookmarkStreamCursor]{
			bookmarkArtworkStream(client, attemptUserID, opts),
			bookmarkNovelStream(client, attemptUserID, opts),
		}
		items, _, _, err := pagination.CollectStreams(ctx, plan, streams)
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
				if err := record.WriteJSONLine(&staged, value); err != nil {
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
			if err := record.WriteJSONValue(&staged, struct {
				Records []record.Record `json:"records"`
			}{Records: records}); err != nil {
				return false, err
			}
		} else {
			for _, item := range items {
				var err error
				switch item.kind {
				case string(target.ResultKindArtwork):
					err = display.PrintArtworks(&staged, []pixiv.Artwork{item.artwork})
				case string(target.ResultKindNovel):
					err = display.PrintNovels(&staged, []pixiv.Novel{item.novel})
				}
				if err != nil {
					return false, err
				}
			}
		}
		_, err = io.Copy(a.env.Out, &staged)
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
				items = append(items, bookmarkListItem{kind: string(target.ResultKindArtwork), artwork: item})
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
				items = append(items, bookmarkListItem{kind: string(target.ResultKindNovel), novel: item})
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
	case string(target.ResultKindArtwork):
		return record.RecordFromArtwork(item.artwork)
	case string(target.ResultKindNovel):
		return record.RecordFromNovel(item.novel)
	default:
		return record.Record{}, fmt.Errorf("unsupported bookmark item kind %q", item.kind)
	}
}

func (a commandRunner) runArtworkList(cmd *cobra.Command, args []string, opts listOptions) error {
	plan, err := environment.ParsePlan(cmd, opts.limit, opts.page)
	if err != nil {
		return err
	}
	var requestedUserID int64
	if len(args) == 1 || opts.input.Mode() == environment.RecordMode {
		requestedUserID, err = a.resolveBookmarkUserID(cmd, args, opts.input, opts.typ, bookmarkListContract())
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
	fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		if userID == 0 {
			userID, err = session.CurrentUserID(client)
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
	return record.RunArtworks(cmd.Context(), a.runner, request, plan, record.FormatOf(jsonOut, ndjson), a.env.Err, record.ArtworkList{Fetch: fetch, Print: func(items []pixiv.Artwork, _ int) error {
		return display.PrintArtworks(a.env.Out, items)
	}})
}

func (a commandRunner) runNovelList(cmd *cobra.Command, args []string, opts listOptions) error {
	plan, err := environment.ParsePlan(cmd, opts.limit, opts.page)
	if err != nil {
		return err
	}
	var requestedUserID int64
	if len(args) == 1 || opts.input.Mode() == environment.RecordMode {
		requestedUserID, err = a.resolveBookmarkUserID(cmd, args, opts.input, opts.typ, bookmarkListContract())
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
	fetch := func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
		if userID == 0 {
			userID, err = session.CurrentUserID(client)
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
	return record.RunNovels(cmd.Context(), a.runner, request, plan, record.FormatOf(jsonOut, ndjson), a.env.Err, record.NovelList{Fetch: fetch, Print: func(items []pixiv.Novel) error {
		return display.PrintNovels(a.env.Out, items)
	}})
}
