package recommended

import (
	"context"
	"fmt"
	"io"

	"github.com/FlanChanXwO/pixiv-cli/internal/display"
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
	runner := session.NewRunner(spool, func(ctx context.Context, _ session.ProxyRequest, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
		_, err := attempt(ctx, client)
		return err
	})
	format := record.FormatOf(jsonOut, false)
	if jsonOut {
		if _, err := io.WriteString(spool, `{"artworks":`); err != nil {
			return false, err
		}
	}
	if jsonOut {
		if err := record.RunArtworks(ctx, runner, session.ProxyRequest{}, plan, format, io.Discard, record.ArtworkList{
			Fetch: func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
				return fetchRecommendedArtworks(client, searchfilter.Filter{})(ctx, cursor)
			},
			Print: func(items []pixiv.Artwork, _ int) error { return nil },
		}); err != nil {
			return false, err
		}
	}
	if !jsonOut {
		items, _, err := pagination.CollectPagesFrom(ctx, plan, sdk.Cursor{}, fetchRecommendedArtworks(client, searchfilter.Filter{}))
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
	if err := record.RunNovels(ctx, runner, session.ProxyRequest{}, plan, format, io.Discard, record.NovelList{
		Fetch: func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
			page, err := client.RecommendedNovels(ctx, pixiv.RecommendedNovelsRequest{Cursor: cursor})
			return page.Items, page.Next, err
		},
		Print: func(items []pixiv.Novel) error { return display.PrintNovels(spool, items) },
	}); err != nil {
		return false, err
	}
	if jsonOut {
		if _, err := io.WriteString(spool, `,"users":`); err != nil {
			return false, err
		}
	}
	if err := record.RunUsers(ctx, runner, session.ProxyRequest{}, plan, format, io.Discard, record.UserList{
		Fetch: func(client *pixiv.Client, ctx context.Context, cursor sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error) {
			page, err := client.RecommendedUsers(ctx, pixiv.RecommendedUsersRequest{Cursor: cursor})
			return page.Items, page.Next, err
		},
		Print: func(items []pixiv.UserPreview) error { return display.PrintUserPreviews(spool, items) },
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
