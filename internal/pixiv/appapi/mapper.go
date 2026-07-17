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
		AIType: dto.AIType, CreateDate: dto.CreateDate, Width: dto.Width, Height: dto.Height, Tools: append([]string(nil), dto.Tools...),
	}
}

func mapNovel(dto novelDTO) model.Novel {
	var tags []model.Tag
	if dto.Tags != nil {
		tags = make([]model.Tag, len(dto.Tags))
	}
	for i, tag := range dto.Tags {
		tags[i] = model.Tag{Name: tag.Name, TranslatedName: tag.TranslatedName}
	}
	return model.Novel{
		ID: dto.ID, Title: dto.Title, Caption: dto.Caption, User: mapUser(dto.User), Tags: tags,
		ImageURLs: mapImageURLs(dto.ImageURLs), CreateDate: dto.CreateDate, TotalBookmarks: dto.TotalBookmarks, TotalView: dto.TotalView,
	}
}

func mapNovelList(dto novelListDTO) model.NovelList {
	var result model.NovelList
	if dto.Novels.Items != nil {
		result.Novels = make([]model.Novel, len(dto.Novels.Items))
	}
	for i, novel := range dto.Novels.Items {
		result.Novels[i] = mapNovel(novel)
	}
	return result
}

func mapRecommendedUserList(dto recommendedUserListDTO) model.RecommendedUserList {
	var result model.RecommendedUserList
	if dto.UserPreviews.Items != nil {
		result.UserPreviews = make([]model.RecommendedUserPreview, len(dto.UserPreviews.Items))
	}
	for i, preview := range dto.UserPreviews.Items {
		out := model.RecommendedUserPreview{User: mapUser(preview.User)}
		if preview.Illusts != nil {
			out.Illusts = make([]model.Illust, len(preview.Illusts))
			for j, illust := range preview.Illusts {
				out.Illusts[j] = mapIllust(illust)
			}
		}
		if preview.Novels != nil {
			out.Novels = make([]model.Novel, len(preview.Novels))
			for j, novel := range preview.Novels {
				out.Novels[j] = mapNovel(novel)
			}
		}
		result.UserPreviews[i] = out
	}
	return result
}

func mapUser(dto userDTO) model.User {
	return model.User{
		ID: dto.ID, Name: dto.Name, Account: dto.Account, Comment: dto.Comment, IsFollowed: dto.IsFollowed,
		ProfileImageURLs: model.ProfileImageURLs{Medium: optionalURL(dto.ProfileImageURLs.Medium)},
	}
}

func mapUserDetail(dto userDetailDTO) model.UserDetail {
	return model.UserDetail{
		User:             mapUser(dto.User.Value),
		Profile:          mapProfile(dto.Profile.Value),
		ProfilePublicity: mapProfilePublicity(dto.ProfilePublicity.Value),
		Workspace:        mapWorkspace(dto.Workspace.Value),
	}
}

func mapProfile(dto profileDTO) model.Profile {
	return model.Profile{
		Webpage: optionalURL(dto.Webpage), Gender: dto.Gender, Birth: dto.Birth, BirthDay: dto.BirthDay, BirthYear: dto.BirthYear,
		Region: dto.Region, AddressID: dto.AddressID, CountryCode: dto.CountryCode, Job: dto.Job, JobID: dto.JobID,
		TotalFollowUsers: dto.TotalFollowUsers, TotalMyPixivUsers: dto.TotalMyPixivUsers, TotalIllusts: dto.TotalIllusts,
		TotalManga: dto.TotalManga, TotalNovels: dto.TotalNovels, TotalIllustBookmarksPublic: dto.TotalIllustBookmarksPublic,
		TotalIllustSeries: dto.TotalIllustSeries, TotalNovelSeries: dto.TotalNovelSeries,
		BackgroundImageURL: optionalURL(dto.BackgroundImageURL), TwitterAccount: dto.TwitterAccount,
		TwitterURL: optionalURL(dto.TwitterURL), PawooURL: optionalURL(dto.PawooURL), IsPremium: dto.IsPremium,
		IsUsingCustomProfileImage: dto.IsUsingCustomProfileImage,
	}
}

func mapProfilePublicity(dto profilePublicityDTO) model.ProfilePublicity {
	return model.ProfilePublicity{
		Gender: dto.Gender.Value, Region: dto.Region.Value, BirthDay: dto.BirthDay.Value,
		BirthYear: dto.BirthYear.Value, Job: dto.Job.Value, Pawoo: dto.Pawoo.Value,
	}
}

func mapWorkspace(dto workspaceDTO) model.Workspace {
	return model.Workspace{
		PC: dto.PC, Monitor: dto.Monitor, Tool: dto.Tool, Scanner: dto.Scanner, Tablet: dto.Tablet, Mouse: dto.Mouse,
		Printer: dto.Printer, Desktop: dto.Desktop, Music: dto.Music, Desk: dto.Desk, Chair: dto.Chair, Comment: dto.Comment,
		WorkspaceImageURL: optionalURL(dto.WorkspaceImageURL),
	}
}

func optionalURL(value *string) *string {
	if value == nil || *value == "" {
		return nil
	}
	return value
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
