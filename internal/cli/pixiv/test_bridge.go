package pixiv

import (
	"context"
	"io"

	pixivapp "github.com/FlanChanXwO/pixiv-cli/internal/application/pixiv"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// RecommendationSpool 暴露推荐临时文档类型别名，供根 CLI 集成测试注入写入
// 失败并验证敏感输出不会提前提交。
type RecommendationSpool = recommendationSpool

// NewRecommendationSpool 暴露推荐临时文档构造器，供根 CLI 集成测试使用。
func NewRecommendationSpool(jsonOut bool) (*RecommendationSpool, error) {
	return newRecommendationSpool(jsonOut)
}

// PrintIllustsForCLI、PrintNovelsForCLI 等函数是根控制器测试的迁移 seam；真实
// 命令构造和输出逻辑仍只在本 package 内使用未导出实现。
func PrintIllustsForCLI(w io.Writer, items []pixiv.Artwork, offset int, ranked bool) error {
	return printIllusts(w, items, offset, ranked)
}

func PrintNovelsForCLI(w io.Writer, items []pixiv.Novel) error { return printNovels(w, items) }

func PrintUserSearchPreviewsForCLI(w io.Writer, items []pixiv.UserPreview) error {
	return printUserSearchPreviews(w, items)
}

func PrintUserDetailForCLI(w io.Writer, result pixiv.UserDetail) error {
	return printUserDetail(w, result)
}

// RunRecommendedAllNDJSONForCLI 只为迁移期根测试提供输出 writer seam。
func RunRecommendedAllNDJSONForCLI(ctx context.Context, out io.Writer, client pixivapp.ClientSet, limit, skip int, oneBatch bool) (bool, error) {
	a := controller{out: out}
	return a.runRecommendedAllNDJSON(ctx, client, listPlan{limit: limit, skip: skip, oneBatch: oneBatch})
}
