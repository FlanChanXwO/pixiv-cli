package user

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/internal/display"
	legacysdk "github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func (a commandRunner) runUsers(ctx context.Context, request session.ProxyRequest, plan pagination.PagePlan, format record.Format, fetch func(context.Context, int64, *pixiv.Client, sdk.Cursor) (sdk.Page[*pixiv.User], error)) error {
	return runList(a, ctx, request, plan, format, fetch, record.UserPreviewOutput, record.List[legacysdk.UserPreview]{
		Print:       func(items []legacysdk.UserPreview, _ int) error { return display.PrintUserPreviews(a.env.Out, items) },
		RecordValue: func(item legacysdk.UserPreview) (any, error) { return record.RecordFromUserPreview(item) },
	})
}

func (a commandRunner) runArtworkList(ctx context.Context, request session.ProxyRequest, plan pagination.PagePlan, format record.Format, fetch func(context.Context, int64, *pixiv.Client, sdk.Cursor) (sdk.Page[*pixiv.Artwork], error)) error {
	return runList(a, ctx, request, plan, format, fetch, func(item *pixiv.Artwork) (legacysdk.Artwork, error) { return record.ArtworkOutput(item, false) }, record.List[legacysdk.Artwork]{
		Print:       func(items []legacysdk.Artwork, _ int) error { return display.PrintArtworks(a.env.Out, items) },
		RecordValue: func(item legacysdk.Artwork) (any, error) { return record.RecordFromArtwork(item) },
	})
}

func (a commandRunner) runNovelList(ctx context.Context, request session.ProxyRequest, plan pagination.PagePlan, format record.Format, fetch func(context.Context, int64, *pixiv.Client, sdk.Cursor) (sdk.Page[*pixiv.Novel], error)) error {
	return runList(a, ctx, request, plan, format, fetch, record.NovelOutput, record.List[legacysdk.Novel]{
		Print:       func(items []legacysdk.Novel, _ int) error { return display.PrintNovels(a.env.Out, items) },
		RecordValue: func(item legacysdk.Novel) (any, error) { return record.RecordFromNovel(item) },
	})
}

// runList 在每次尝试中投影，复用旧输出格式与新 SDK 的账号/分页边界。
func runList[T any, O any](a commandRunner, ctx context.Context, request session.ProxyRequest, plan pagination.PagePlan, format record.Format, fetch func(context.Context, int64, *pixiv.Client, sdk.Cursor) (sdk.Page[T], error), project func(T) (O, error), output record.List[O]) error {
	return record.RunSDKList(ctx, a.env.Out, a.env.Err, a.executeSDK, request, plan, format, output, func(ctx context.Context, selected int64, client *pixiv.Client, cursor sdk.Cursor) ([]O, sdk.Cursor, error) {
		page, err := fetch(ctx, selected, client, cursor)
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		items := make([]O, len(page.Items))
		for i, item := range page.Items {
			items[i], err = project(item)
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
		}
		return items, page.Cursor, nil
	})
}
