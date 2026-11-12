package recommended

import (
	"context"
	"fmt"
	"io"

	"github.com/FlanChanXwO/pixiv-cli/internal/display"
	legacysdk "github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/searchfilter"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// runAll 在同一次账号尝试内暂存全部分区，完成后才提交；文本保留插画、漫画的顺序。
func (a commandRunner) runAll(ctx context.Context, client *pixiv.Client, plan pagination.PagePlan, jsonOut bool) (bool, error) {
	spool, err := record.NewSpool()
	if err != nil {
		return false, err
	}
	defer spool.Close()
	execute := func(ctx context.Context, _ session.ProxyRequest, attempt func(context.Context, int64, *pixiv.Client) (bool, error)) error {
		_, err := attempt(ctx, 0, client)
		return err
	}
	format := record.FormatOf(jsonOut, false)
	if jsonOut {
		if _, err := io.WriteString(spool, `{"artworks":`); err != nil {
			return false, err
		}
		if err := record.RunSDKList(ctx, spool, io.Discard, execute, session.ProxyRequest{}, plan, format, record.List[legacysdk.Artwork]{
			RecordValue: func(item legacysdk.Artwork) (any, error) { return record.RecordFromArtwork(item) },
		}, func(ctx context.Context, _ int64, client *pixiv.Client, cursor sdk.Cursor) ([]legacysdk.Artwork, sdk.Cursor, error) {
			return fetchRecommendedArtworks(client)(ctx, cursor)
		}); err != nil {
			return false, err
		}
	} else {
		items, _, err := pagination.CollectPagesFrom(ctx, plan, sdk.Cursor{}, fetchRecommendedArtworks(client))
		if err != nil {
			return false, err
		}
		for _, contentType := range []searchfilter.ContentType{searchfilter.ContentTypeIllust, searchfilter.ContentTypeManga} {
			if err := display.PrintArtworks(spool, filterRecommendedArtworks(items, searchfilter.Filter{ContentType: contentType})); err != nil {
				return false, err
			}
		}
	}
	if jsonOut {
		if _, err := io.WriteString(spool, `,"novels":`); err != nil {
			return false, err
		}
	}
	if err := record.RunSDKList(ctx, spool, io.Discard, execute, session.ProxyRequest{}, plan, format, record.List[legacysdk.Novel]{
		Print:       func(items []legacysdk.Novel, _ int) error { return display.PrintNovels(spool, items) },
		RecordValue: func(item legacysdk.Novel) (any, error) { return record.RecordFromNovel(item) },
	}, func(ctx context.Context, _ int64, client *pixiv.Client, cursor sdk.Cursor) ([]legacysdk.Novel, sdk.Cursor, error) {
		return fetchRecommendedNovels(client)(ctx, cursor)
	}); err != nil {
		return false, err
	}

	if jsonOut {
		if _, err := io.WriteString(spool, `,"users":`); err != nil {
			return false, err
		}
	}
	if err := record.RunSDKList(ctx, spool, io.Discard, execute, session.ProxyRequest{}, plan, format, record.List[legacysdk.UserPreview]{
		Print:       func(items []legacysdk.UserPreview, _ int) error { return display.PrintUserPreviews(spool, items) },
		RecordValue: func(item legacysdk.UserPreview) (any, error) { return record.RecordFromUserPreview(item) },
	}, func(ctx context.Context, _ int64, client *pixiv.Client, cursor sdk.Cursor) ([]legacysdk.UserPreview, sdk.Cursor, error) {
		return fetchRecommendedUsers(client)(ctx, cursor)
	}); err != nil {
		return false, err
	}

	if jsonOut {
		if _, err := io.WriteString(spool, "}\n"); err != nil {
			return false, err
		}
	} else {
		info, err := spool.Stat()
		if err != nil {
			return false, err
		}
		if info.Size() == 0 {
			_, err := fmt.Fprintln(a.env.Err, "No results.")
			return false, err
		}
	}
	return true, spool.Commit(a.env.Out)
}
