package record

import (
	legacysdk "github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// ArtworkOutput 保留 CLI/MCP 的作品输出契约；detail 控制是否交付图片页。
// 旧模型只作为输出值，绑定对象在本次 SDK 操作内投影后即可释放。
func ArtworkOutput(a *pixiv.Artwork, detail bool) (legacysdk.Artwork, error) {
	user, err := UserOutput(a.Author)
	if err != nil {
		return legacysdk.Artwork{}, err
	}
	out := legacysdk.Artwork{
		ID: a.ID, Title: a.Title, Caption: a.Caption, ContentType: legacysdk.ContentType(a.ContentType),
		User: user, Tags: outputTags(a.Tags), PublishedAt: a.PublishedAt,
		TotalBookmarks: a.TotalBookmarks, TotalViews: a.TotalViews, Width: a.Width, Height: a.Height,
		PageCount: a.PageCount, XRestrict: a.XRestrict, AIType: a.AIType, Tools: a.Tools,
		IsBookmarked: a.IsBookmarked, IsMuted: a.IsMuted, Visible: a.Visible, SanityLevel: a.SanityLevel,
		RestrictionAttributes: a.RestrictionAttributes, TotalComments: a.TotalComments,
	}
	if a.Series != nil {
		out.Series = &legacysdk.ArtworkSeriesSummary{ID: a.Series.ID, Title: a.Series.Title}
	}
	out.Cover, err = outputImage(a.Cover, "artwork", a.ID, -1, a.Cover.Variant())
	if err != nil {
		return legacysdk.Artwork{}, err
	}
	if detail {
		for _, page := range a.Pages {
			if a.ContentType == pixiv.ContentTypeUgoira && page.Original.Variant() != "original" {
				continue
			}
			image, err := outputImage(page.Original, "artwork", a.ID, page.PageIndex, "original")
			if err != nil {
				return legacysdk.Artwork{}, err
			}
			out.Pages = append(out.Pages, legacysdk.ArtworkPage{PageIndex: page.PageIndex, Image: image, Width: page.Width, Height: page.Height})
		}
	}
	return out, nil
}

// UserOutput 将新 SDK 的用户摘要投影为既有输出值。
func UserOutput(u *pixiv.User) (legacysdk.User, error) {
	image, err := outputImage(u.ProfileImage, "user_profile", u.ID, -1, "medium")
	return legacysdk.User{ID: u.ID, Name: u.Name, Account: u.Account, Comment: u.Comment, IsFollowed: u.IsFollowed, ProfileImage: image}, err
}

// NovelOutput 为聚合推荐和收藏保留小说输出；独立小说命令继续使用旧入口。
func NovelOutput(n *pixiv.Novel) (legacysdk.Novel, error) {
	u, err := UserOutput(n.Author)
	if err != nil {
		return legacysdk.Novel{}, err
	}
	cover, err := outputImage(n.Cover, "novel_cover", n.ID, -1, n.Cover.Variant())
	return legacysdk.Novel{ID: n.ID, Title: n.Title, Caption: n.Caption, User: u, Tags: outputTags(n.Tags),
		PublishedAt: n.PublishedAt, XRestrict: n.XRestrict, TextLength: n.TextLength, IsOriginal: n.IsOriginal,
		TotalBookmarks: n.TotalBookmarks, TotalViews: n.TotalViews, Cover: cover}, err
}

// RecordFromSDKArtwork 将新 SDK 作品投影为既有管道记录。
func RecordFromSDKArtwork(a *pixiv.Artwork, detail bool) (Record, error) {
	value, err := ArtworkOutput(a, detail)
	if err != nil {
		return Record{}, err
	}
	return RecordFromArtwork(value)
}

// FromSDKArtworks 保留列表记录省略 pages 的契约。
func FromSDKArtworks(items []*pixiv.Artwork) ([]Record, error) {
	return recordsFrom(items, func(a *pixiv.Artwork) (Record, error) { return RecordFromSDKArtwork(a, false) })
}

// FromSDKNovels 为新 SDK 聚合列表构造小说记录。
func FromSDKNovels(items []*pixiv.Novel) ([]Record, error) {
	return recordsFrom(items, func(n *pixiv.Novel) (Record, error) {
		value, err := NovelOutput(n)
		if err != nil {
			return Record{}, err
		}
		return RecordFromNovel(value)
	})
}

// FromSDKRecommendations 保留推荐用户及其样例作品的记录形状。
func FromSDKRecommendations(items []pixiv.UserRecommendation) ([]Record, error) {
	return recordsFrom(items, func(r pixiv.UserRecommendation) (Record, error) {
		out, err := UserPreviewOutput(r.User)
		if err != nil {
			return Record{}, err
		}
		for _, a := range r.Artworks {
			value, err := ArtworkOutput(a, false)
			if err != nil {
				return Record{}, err
			}
			out.Illusts = append(out.Illusts, value)
		}
		for _, n := range r.Novels {
			value, err := NovelOutput(n)
			if err != nil {
				return Record{}, err
			}
			out.Novels = append(out.Novels, value)
		}
		return RecordFromUserPreview(out)
	})
}

func outputTags(tags []pixiv.Tag) []legacysdk.Tag {
	out := make([]legacysdk.Tag, len(tags))
	for i, tag := range tags {
		out[i] = legacysdk.Tag{Name: tag.Name, TranslatedName: tag.TranslatedName}
	}
	return out
}

// outputImage 只序列化稳定身份，沿用旧资源协议供尚未迁移的媒体工具解析。
func outputImage(image pixiv.Image, kind string, id int64, page int, variant string) (legacysdk.ImageResource, error) {
	if image.URL == "" {
		return legacysdk.ImageResource{}, nil
	}
	resource, err := outputResource(kind, id, page, variant)
	return legacysdk.ImageResource{Resource: resource, Variant: variant, Width: image.Width, Height: image.Height}, err
}
