package record

import (
	"context"
	"io"

	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// RunSDKList 让新 SDK 共用列表输出与提交边界；每次尝试使用自己的已选 UID。
func RunSDKList[T any](ctx context.Context, out, errOut io.Writer, execute session.SDKExecutor, request session.ProxyRequest, plan pagination.PagePlan, format Format, list List[T], fetch func(context.Context, int64, *pixiv.Client, sdk.Cursor) ([]T, sdk.Cursor, error)) error {
	var selected int64
	run := func(ctx context.Context, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
		return execute(ctx, request, func(ctx context.Context, id int64, client *pixiv.Client) (bool, error) {
			selected = id
			return attempt(ctx, client)
		})
	}
	return runList(ctx, out, errOut, run, plan, format, list, func(ctx context.Context, client *pixiv.Client, cursor sdk.Cursor) ([]T, sdk.Cursor, error) {
		return fetch(ctx, selected, client, cursor)
	})
}
