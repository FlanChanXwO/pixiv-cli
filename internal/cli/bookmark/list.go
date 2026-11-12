package bookmark

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/FlanChanXwO/pixiv-cli/internal/display"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	legacysdk "github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/target"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
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

	return a.executeSDK(cmd.Context(), request, func(ctx context.Context, selected int64, client *pixiv.Client) (bool, error) {
		attemptUserID := userID
		if attemptUserID == 0 {
			attemptUserID = selected
		}
		streams := []pagination.Stream[bookmarkListItem, bookmarkStreamCursor]{
			bookmarkArtworkStream(client, attemptUserID, opts),
			bookmarkNovelStream(client, attemptUserID, opts),
		}
		items, _, page, err := pagination.CollectStreams(ctx, plan, streams)
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
			if err := record.WriteJSONValue(&staged, record.NewListOutput(records, plan, page)); err != nil {
				return false, err
			}
		} else {
			for _, item := range items {
				var err error
				switch item.kind {
				case string(target.NamespaceArtwork):
					err = display.PrintArtworks(&staged, []legacysdk.Artwork{item.artwork})
				case string(target.NamespaceNovel):
					err = display.PrintNovels(&staged, []legacysdk.Novel{item.novel})
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
			result, err := client.User(userID).ArtworkBookmarks(ctx, pixiv.ArtworkBookmarksOptions{
				Restrict: pixiv.Restrict(opts.restrict), Tag: opts.tag, Cursor: cursor.upstream,
			})
			if err != nil {
				return nil, bookmarkStreamCursor{}, err
			}
			if cursor.consumed > len(result.Items) {
				return nil, bookmarkStreamCursor{}, errors.New("bookmark artwork checkpoint exceeds upstream batch")
			}
			items := make([]bookmarkListItem, 0, len(result.Items)-cursor.consumed)
			for _, item := range result.Items[cursor.consumed:] {
				value, err := record.ArtworkOutput(item, false)
				if err != nil {
					return nil, bookmarkStreamCursor{}, err
				}
				items = append(items, bookmarkListItem{kind: string(target.NamespaceArtwork), artwork: value})
			}
			return items, bookmarkStreamCursor{upstream: result.Cursor}, nil
		},
		Checkpoint: bookmarkCheckpoint,
	}
}

func bookmarkNovelStream(client *pixiv.Client, userID int64, opts listOptions) pagination.Stream[bookmarkListItem, bookmarkStreamCursor] {
	return pagination.Stream[bookmarkListItem, bookmarkStreamCursor]{
		Fetch: func(ctx context.Context, cursor bookmarkStreamCursor) ([]bookmarkListItem, bookmarkStreamCursor, error) {
			result, err := client.User(userID).NovelBookmarks(ctx, pixiv.NovelBookmarksOptions{
				Restrict: pixiv.Restrict(opts.restrict), Tag: opts.tag, Cursor: cursor.upstream,
			})
			if err != nil {
				return nil, bookmarkStreamCursor{}, err
			}
			if cursor.consumed > len(result.Items) {
				return nil, bookmarkStreamCursor{}, errors.New("bookmark novel checkpoint exceeds upstream batch")
			}
			items := make([]bookmarkListItem, 0, len(result.Items)-cursor.consumed)
			for _, item := range result.Items[cursor.consumed:] {
				value, err := record.NovelOutput(item)
				if err != nil {
					return nil, bookmarkStreamCursor{}, err
				}
				items = append(items, bookmarkListItem{kind: string(target.NamespaceNovel), novel: value})
			}
			return items, bookmarkStreamCursor{upstream: result.Cursor}, nil
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
	case string(target.NamespaceArtwork):
		return record.RecordFromArtwork(item.artwork)
	case string(target.NamespaceNovel):
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
	fetch := func(ctx context.Context, selected int64, client *pixiv.Client, cursor sdk.Cursor) ([]legacysdk.Artwork, sdk.Cursor, error) {
		userID := requestedUserID
		if userID == 0 {
			userID = selected
		}
		result, err := client.User(userID).ArtworkBookmarks(ctx, pixiv.ArtworkBookmarksOptions{Restrict: pixiv.Restrict(opts.restrict), Tag: opts.tag, Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		items := make([]legacysdk.Artwork, len(result.Items))
		for i, item := range result.Items {
			items[i], err = record.ArtworkOutput(item, false)
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
		}
		return items, result.Cursor, nil
	}
	return record.RunSDKList(cmd.Context(), a.env.Out, a.env.Err, a.executeSDK, request, plan, record.FormatOf(jsonOut, ndjson), record.List[legacysdk.Artwork]{
		RecordValue: func(item legacysdk.Artwork) (any, error) { return record.RecordFromArtwork(item) },
		Print:       func(items []legacysdk.Artwork, _ int) error { return display.PrintArtworks(a.env.Out, items) },
	}, fetch)
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
	fetch := func(ctx context.Context, selected int64, client *pixiv.Client, cursor sdk.Cursor) ([]legacysdk.Novel, sdk.Cursor, error) {
		userID := requestedUserID
		if userID == 0 {
			userID = selected
		}
		result, err := client.User(userID).NovelBookmarks(ctx, pixiv.NovelBookmarksOptions{Restrict: pixiv.Restrict(opts.restrict), Tag: opts.tag, Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		items := make([]legacysdk.Novel, len(result.Items))
		for i, item := range result.Items {
			items[i], err = record.NovelOutput(item)
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
		}
		return items, result.Cursor, nil
	}
	return record.RunSDKList(cmd.Context(), a.env.Out, a.env.Err, a.executeSDK, request, plan, record.FormatOf(jsonOut, ndjson), record.List[legacysdk.Novel]{
		RecordValue: func(item legacysdk.Novel) (any, error) { return record.RecordFromNovel(item) },
		Print:       func(items []legacysdk.Novel, _ int) error { return display.PrintNovels(a.env.Out, items) },
	}, fetch)
}
