package appapi

import "github.com/FlanChanXwO/pixiv-cli/internal/pixiv/model"

func mapIllustList(dto illustListDTO) model.IllustList {
	var result model.IllustList
	if dto.Illusts.Items != nil {
		result.Illusts = make([]model.Illust, len(dto.Illusts.Items))
	}
	for i, item := range dto.Illusts.Items {
		result.Illusts[i] = mapIllust(item)
	}
	return result
}

func mapIllustDetail(dto illustDetailDTO) model.IllustDetail {
	return model.IllustDetail{Illust: mapIllust(*dto.Illust)}
}

func mapIllust(dto illustDTO) model.Illust {
	var tags []model.Tag
	if dto.Tags != nil {
		tags = make([]model.Tag, len(dto.Tags))
	}
	for i, tag := range dto.Tags {
		tags[i] = model.Tag{Name: tag.Name, TranslatedName: tag.TranslatedName}
	}
	var pages []model.MetaPage
	if dto.MetaPages != nil {
		pages = make([]model.MetaPage, len(dto.MetaPages))
	}
	for i, page := range dto.MetaPages {
		pages[i] = mapMetaPage(page)
	}
	return model.Illust{
		ID: dto.ID, Title: dto.Title, Type: dto.Type, PageCount: dto.PageCount,
		TotalBookmarks: dto.TotalBookmarks, TotalView: dto.TotalView, XRestrict: dto.XRestrict,
		User: mapUser(dto.User), Tags: tags, ImageURLs: mapImageURLs(dto.ImageURLs),
		MetaSinglePage: model.SinglePage{OriginalImageURL: dto.MetaSinglePage.OriginalImageURL}, MetaPages: pages,
		AIType: dto.AIType, CreateDate: dto.CreateDate, Width: dto.Width, Height: dto.Height,
	}
}

func mapUser(dto userDTO) model.User {
	return model.User{ID: dto.ID, Name: dto.Name, Account: dto.Account, Comment: dto.Comment, IsFollowed: dto.IsFollowed}
}
func mapImageURLs(dto imageURLsDTO) model.ImageURLs {
	return model.ImageURLs{SquareMedium: dto.SquareMedium, Medium: dto.Medium, Large: dto.Large, Original: dto.Original}
}
func mapMetaPage(dto metaPageDTO) model.MetaPage {
	return model.MetaPage{PageIndex: dto.PageIndex, Width: dto.Width, Height: dto.Height, Extension: dto.Extension, ImageURLs: mapImageURLs(dto.ImageURLs)}
}

func mapUserPreviewList(dto userPreviewListDTO) model.UserPreviewList {
	var result model.UserPreviewList
	if dto.UserPreviews.Items != nil {
		result.UserPreviews = make([]model.UserPreview, len(dto.UserPreviews.Items))
	}
	for i, preview := range dto.UserPreviews.Items {
		result.UserPreviews[i] = model.UserPreview{User: mapUser(preview.User)}
	}
	return result
}

func mapTrendTags(dto trendTagsDTO) model.TrendTags {
	var result model.TrendTags
	if dto.TrendTags.Items != nil {
		result.TrendTags = make([]model.TrendTag, len(dto.TrendTags.Items))
	}
	for i, trend := range dto.TrendTags.Items {
		result.TrendTags[i] = model.TrendTag{Tag: trend.Tag, TranslatedName: trend.TranslatedName, Illust: mapIllust(trend.Illust.Value)}
	}
	return result
}

func mapUgoiraMetadata(dto ugoiraMetadataResultDTO) model.UgoiraMetadataResult {
	var result model.UgoiraMetadataResult
	metadata := dto.UgoiraMetadata.Value
	result.UgoiraMetadata.ZipURLs.Medium = metadata.ZipURLs.Value.Medium
	if metadata.Frames.Items != nil {
		result.UgoiraMetadata.Frames = make([]model.UgoiraFrame, len(metadata.Frames.Items))
	}
	for i, frame := range metadata.Frames.Items {
		result.UgoiraMetadata.Frames[i] = model.UgoiraFrame{File: frame.File, Delay: frame.Delay}
	}
	return result
}
