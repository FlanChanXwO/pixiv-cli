package recommended

import (
	"context"
	"fmt"
	"io"

	"github.com/FlanChanXwO/pixiv-cli/internal/display"
	"github.com/FlanChanXwO/pixiv-cli/internal/environment"
	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/searchfilter"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
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
	execute := func(ctx context.Context, _ environment.PixivRequest, attempt func(context.Context, int64, *pixiv.Client) (bool, error)) error {
		_, err := attempt(ctx, 0, client)
		return err
	}
	format := record.FormatOf(jsonOut, false)
	if jsonOut {
		if _, err := io.WriteString(spool, `{"artworks":`); err != nil {
			return false, err
		}
		if err := record.RunSDKList(ctx, spool, io.Discard, execute, environment.PixivRequest{}, plan, format, record.List[record.Artwork]{
			RecordValue: func(item record.Artwork) (any, error) { return record.RecordFromArtwork(item) },
		}, func(ctx context.Context, _ int64, client *pixiv.Client, cursor sdk.Cursor) ([]record.Artwork, sdk.Cursor, error) {
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
			if err := display.PrintArtworks(spool, a.env.Language, filterRecommendedArtworks(items, searchfilter.Filter{ContentType: contentType})); err != nil {
				return false, err
			}
		}
	}
	if jsonOut {
		if _, err := io.WriteString(spool, `,"novels":`); err != nil {
			return false, err
		}
	}
	if err := record.RunSDKList(ctx, spool, io.Discard, execute, environment.PixivRequest{}, plan, format, record.List[record.Novel]{
		Print:       func(items []record.Novel, _ int) error { return display.PrintNovels(spool, a.env.Language, items) },
		RecordValue: func(item record.Novel) (any, error) { return record.RecordFromNovel(item) },
	}, func(ctx context.Context, _ int64, client *pixiv.Client, cursor sdk.Cursor) ([]record.Novel, sdk.Cursor, error) {
		return fetchRecommendedNovels(client)(ctx, cursor)
	}); err != nil {
		return false, err
	}

	if jsonOut {
		if _, err := io.WriteString(spool, `,"users":`); err != nil {
			return false, err
		}
	}
	if err := record.RunSDKList(ctx, spool, io.Discard, execute, environment.PixivRequest{}, plan, format, record.List[record.UserPreview]{
		Print:       func(items []record.UserPreview, _ int) error { return display.PrintUserPreviews(spool, items) },
		RecordValue: func(item record.UserPreview) (any, error) { return record.RecordFromUserPreview(item) },
	}, func(ctx context.Context, _ int64, client *pixiv.Client, cursor sdk.Cursor) ([]record.UserPreview, sdk.Cursor, error) {
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
			_, err := fmt.Fprintln(a.env.Err, a.env.Text("output.no_results"))
			return false, err
		}
	}
	return true, spool.Commit(a.env.Out)
}
