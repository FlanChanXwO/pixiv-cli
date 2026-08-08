package cli

import (
	"context"
	"io"

	downloadapp "github.com/FlanChanXwO/pixiv-cli/internal/application/download"
	paginationapp "github.com/FlanChanXwO/pixiv-cli/internal/application/pagination"
	pixivapp "github.com/FlanChanXwO/pixiv-cli/internal/application/pixiv"
	downloadcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/download"
	pixivcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// listPlan/pageItems 保留根 CLI 历史测试的分页断言入口；生产分页实现属于
// internal/cli/pixiv，根包不再持有这些业务 helper。
type listPlan struct {
	limit    int
	skip     int
	oneBatch bool
}

func pageItems[T any](ctx context.Context, plan listPlan, fetch func(context.Context, sdk.Cursor) ([]T, sdk.Cursor, error), consume func([]T) error) error {
	_, err := paginationapp.TraversePages(ctx, paginationapp.PagePlan{
		Skip: plan.skip, Limit: plan.limit, OneBatch: plan.oneBatch,
	}, fetch, consume)
	return err
}

func (a app) runRecommendedAllNDJSON(ctx context.Context, client pixivapp.ClientSet, plan listPlan) (bool, error) {
	return pixivcommands.RunRecommendedAllNDJSONForCLI(ctx, a.out, client, plan.limit, plan.skip, plan.oneBatch)
}

func printIllusts(w io.Writer, items []pixiv.Artwork, offset int, ranked bool) error {
	return pixivcommands.PrintIllustsForCLI(w, items, offset, ranked)
}

func printNovels(w io.Writer, items []pixiv.Novel) error {
	return pixivcommands.PrintNovelsForCLI(w, items)
}

func printUserSearchPreviews(w io.Writer, items []pixiv.UserPreview) error {
	return pixivcommands.PrintUserSearchPreviewsForCLI(w, items)
}

func printUserDetail(w io.Writer, result pixiv.UserDetail) error {
	return pixivcommands.PrintUserDetailForCLI(w, result)
}

func downloadReportError(report downloadapp.DownloadReport) error {
	return downloadcommands.DownloadReportErrorForCLI(report)
}

func downloadReportCommitted(report downloadapp.DownloadReport) bool {
	return downloadcommands.DownloadReportCommittedForCLI(report)
}
