package recommended

import (
	"context"
	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/searchfilter"
	"github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// runAll 按既有类别顺序把四个推荐分区写入一份私有临时文档，只有全部读取成功后才
// 把完整文档交给 stdout。因此中途失败不会让用户看到半份结果。
func (a commandRunner) runAll(ctx context.Context, client *pixiv.Client, plan pagination.PagePlan, jsonOut bool) (bool, error) {
	spool, err := newSpool(jsonOut)
	if err != nil {
		return false, err
	}
	defer spool.Close()

	var visualItems []pixiv.Artwork
	if err := pagination.PageItems(ctx, plan, fetchRecommendedArtworks(client, searchfilter.Filter{}), func(items []pixiv.Artwork) error {
		visualItems = append(visualItems, items...)
		return nil
	}); err != nil {
		return false, err
	}

	for _, section := range []struct {
		key     string
		heading string
		filter  searchfilter.Filter
	}{{key: string(record.JSONKeyIllusts), heading: "recommended illustrations", filter: searchfilter.Filter{ContentType: searchfilter.ContentTypeIllust}}, {key: string(record.JSONKeyManga), heading: "recommended manga", filter: searchfilter.Filter{ContentType: searchfilter.ContentTypeManga}}} {
		if err := spool.section(section.key); err != nil {
			return false, err
		}
		if err := spool.heading(section.heading); err != nil {
			return false, err
		}
		if err := spool.artworks(filterRecommendedArtworks(visualItems, section.filter)); err != nil {
			return false, err
		}
	}
	if err := spool.section("novels"); err != nil {
		return false, err
	}
	if err := spool.heading("recommended novels"); err != nil {
		return false, err
	}
	if err := pagination.PageItems(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.Novel, sdk.Cursor, error) {
		result, err := client.RecommendedNovels(ctx, pixiv.RecommendedNovelsRequest{Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}, spool.novels); err != nil {
		return false, err
	}
	if err := spool.section("user_previews"); err != nil {
		return false, err
	}
	if err := spool.heading("recommended users"); err != nil {
		return false, err
	}
	if err := pagination.PageItems(ctx, plan, func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.UserPreview, sdk.Cursor, error) {
		result, err := client.RecommendedUsers(ctx, pixiv.RecommendedUsersRequest{Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}, spool.users); err != nil {
		return false, err
	}
	// 最终 io.Copy 可能已交付部分 stdout；在尝试前关闭账号池重放窗口。
	return true, spool.Commit(a.env.Out)
}
