package record

import (
	legacysdk "github.com/FlanChanXwO/pixiv-cli/internal/legacy/sdk/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// UserDetailOutput 保留用户详情的 profile/profile_publicity/workspace 布局。
func UserDetailOutput(p *pixiv.Profile) (legacysdk.UserDetail, error) {
	u, err := UserOutput(p.User)
	return legacysdk.UserDetail{User: u,
		Profile: legacysdk.UserProfile{
			Webpage: p.Webpage, Gender: p.Gender, BirthDay: p.BirthDay, BirthYear: p.BirthYear,
			Region: p.Region, CountryCode: p.CountryCode, Job: p.Job,
			TotalFollowUsers: p.TotalFollowUsers, TotalMyPixivUsers: p.TotalMyPixivUsers,
			TotalIllusts: p.TotalIllusts, TotalManga: p.TotalManga, TotalNovels: p.TotalNovels,
			TotalIllustBookmarks: p.TotalIllustBookmarks, TotalIllustSeries: p.TotalIllustSeries, TotalNovelSeries: p.TotalNovelSeries,
			BackgroundImageURL: p.BackgroundImageURL, TwitterAccount: p.TwitterAccount,
			TwitterURL: p.TwitterURL, PawooURL: p.PawooURL,
			IsPremium: p.IsPremium, IsUsingCustomProfileImage: p.IsUsingCustomProfileImage,
		},
		ProfilePublicity: legacysdk.UserProfilePublicity(p.Publicity),
		Workspace:        legacysdk.UserWorkspace(p.Workspace),
	}, err
}

// UserPreviewOutput 为用户列表保留空的 illusts/novels 数组。
func UserPreviewOutput(u *pixiv.User) (legacysdk.UserPreview, error) {
	value, err := UserOutput(u)
	return legacysdk.UserPreview{User: value, Illusts: []legacysdk.Artwork{}, Novels: []legacysdk.Novel{}}, err
}

// RecordFromSDKProfile 把新 SDK 的完整用户详情投影为管道记录。
func RecordFromSDKProfile(p *pixiv.Profile) (Record, error) {
	value, err := UserDetailOutput(p)
	if err != nil {
		return Record{}, err
	}
	return RecordFromUserDetail(value)
}

// FromSDKUsers 将普通用户列表投影为既有预览记录。
func FromSDKUsers(items []*pixiv.User) ([]Record, error) {
	return recordsFrom(items, func(u *pixiv.User) (Record, error) {
		value, err := UserPreviewOutput(u)
		if err != nil {
			return Record{}, err
		}
		return RecordFromUserPreview(value)
	})
}
