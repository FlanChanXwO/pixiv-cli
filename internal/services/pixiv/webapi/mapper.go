package webapi

import (
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/model"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/text"
)

func mapSearchIllust(item webSearchIllust) model.Illust {
	tags := make([]model.Tag, 0, len(item.Tags))
	for _, tag := range item.Tags {
		tags = append(tags, model.Tag{Name: tag})
	}
	imageURLs := model.ImageURLs{SquareMedium: item.URL, Medium: item.URL}
	return model.Illust{
		URL:       artworkURL(int64(item.ID)),
		ID:        int64(item.ID),
		Title:     item.Title,
		Type:      illustType(int(item.IllustType)),
		PageCount: int(item.PageCount),
		XRestrict: int(item.XRestrict),
		User: model.User{
			ID:      int64(item.UserID),
			Name:    item.UserName,
			Account: strconv.FormatInt(int64(item.UserID), 10),
		},
		Tags:      tags,
		ImageURLs: imageURLs,
		AIType:    int(item.AIType),
	}
}

func mapDetailIllust(item webIllustDetail, metaPages []model.MetaPage) model.Illust {
	tags := mapDetailTags(item.Tags.Tags)
	imageURLs := mapDetailURLs(item.URLs)
	if len(metaPages) > 0 {
		imageURLs = firstImageURLs(imageURLs, metaPages[0].ImageURLs)
	}
	pageCount := int(item.PageCount)
	if pageCount == 0 && len(metaPages) > 0 {
		pageCount = len(metaPages)
	}
	illust := model.Illust{
		URL:            artworkURL(int64(firstFlexInt64(item.ID, item.IllustID))),
		ID:             int64(firstFlexInt64(item.ID, item.IllustID)),
		Title:          text.FirstNonEmpty(item.Title, item.IllustTitle),
		Caption:        item.Description,
		Type:           illustType(int(item.IllustType)),
		PageCount:      pageCount,
		TotalBookmarks: int(item.BookmarkCount),
		TotalView:      int(item.ViewCount),
		XRestrict:      int(item.XRestrict),
		User: model.User{
			ID:      int64(item.UserID),
			Name:    item.UserName,
			Account: strconv.FormatInt(int64(item.UserID), 10),
		},
		Tags:       tags,
		ImageURLs:  imageURLs,
		MetaPages:  metaPages,
		AIType:     int(item.AIType),
		CreateDate: item.CreateDate,
		Width:      int(item.Width),
		Height:     int(item.Height),
	}
	if len(metaPages) == 1 {
		illust.MetaSinglePage.OriginalImageURL = metaPages[0].ImageURLs.Original
	}
	return illust
}

func imageExtension(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(path.Ext(parsed.Path), ".")
}

func mapRankingIllust(item webRankingItem) model.Illust {
	tags := make([]model.Tag, 0, len(item.Tags))
	for _, tag := range item.Tags {
		tags = append(tags, model.Tag{Name: tag})
	}
	return model.Illust{
		URL:            artworkURL(int64(item.IllustID)),
		ID:             int64(item.IllustID),
		Title:          item.Title,
		Type:           illustType(int(item.IllustType)),
		PageCount:      int(item.IllustPageCount),
		TotalBookmarks: int(item.RatingCount),
		TotalView:      int(item.ViewCount),
		User: model.User{
			ID:      int64(item.UserID),
			Name:    item.UserName,
			Account: strconv.FormatInt(int64(item.UserID), 10),
		},
		Tags: tags,
		ImageURLs: model.ImageURLs{
			SquareMedium: item.URL,
			Medium:       item.URL,
			Large:        item.URL,
		},
	}
}

func mapDetailTags(items []webTag) []model.Tag {
	tags := make([]model.Tag, 0, len(items))
	for _, item := range items {
		tags = append(tags, model.Tag{Name: item.Tag, TranslatedName: item.Translation.En})
	}
	return tags
}

func mapDetailURLs(urls webDetailURLs) model.ImageURLs {
	return model.ImageURLs{
		SquareMedium: text.FirstNonEmpty(urls.Mini, urls.ThumbMini, urls.Small),
		Medium:       text.FirstNonEmpty(urls.Small, urls.Regular),
		Large:        text.FirstNonEmpty(urls.Regular, urls.Original),
		Original:     urls.Original,
	}
}

func mapPageURLs(urls webPageURLs) model.ImageURLs {
	return model.ImageURLs{
		SquareMedium: urls.ThumbMini,
		Medium:       text.FirstNonEmpty(urls.Small, urls.Regular),
		Large:        text.FirstNonEmpty(urls.Regular, urls.Original),
		Original:     urls.Original,
	}
}

func firstImageURLs(primary, fallback model.ImageURLs) model.ImageURLs {
	return model.ImageURLs{
		SquareMedium: text.FirstNonEmpty(primary.SquareMedium, fallback.SquareMedium),
		Medium:       text.FirstNonEmpty(primary.Medium, fallback.Medium),
		Large:        text.FirstNonEmpty(primary.Large, fallback.Large),
		Original:     text.FirstNonEmpty(primary.Original, fallback.Original),
	}
}

func illustType(value int) string {
	switch value {
	case 1:
		return string(model.IllustTypeManga)
	case 2:
		return string(model.IllustTypeUgoira)
	default:
		return string(model.IllustTypeIllust)
	}
}

func firstFlexInt64(values ...flexInt64) flexInt64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func artworkURL(id int64) string {
	if id <= 0 {
		return ""
	}
	return "https://www.pixiv.net/artworks/" + fmt.Sprint(id)
}
