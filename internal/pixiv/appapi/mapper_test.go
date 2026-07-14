package appapi

import (
	"reflect"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/model"
)

func TestMapIllustPreservesEveryNormalizedField(t *testing.T) {
	dto := illustDTO{
		ID: 1, Title: "title", Type: "manga", PageCount: 2, TotalBookmarks: 3, TotalView: 4, XRestrict: 1,
		User:           userDTO{ID: 5, Name: "name", Account: "account", Comment: "comment", IsFollowed: true},
		Tags:           []tagDTO{{Name: "tag", TranslatedName: "translated"}},
		ImageURLs:      imageURLsDTO{SquareMedium: "square", Medium: "medium", Large: "large", Original: "original"},
		MetaSinglePage: singlePageDTO{OriginalImageURL: "single"},
		MetaPages:      []metaPageDTO{{PageIndex: 1, Width: 6, Height: 7, Extension: "png", ImageURLs: imageURLsDTO{Original: "page"}}},
		AIType:         2, CreateDate: "2026-07-12", Width: 8, Height: 9,
	}
	want := model.Illust{
		ID: 1, Title: "title", Type: "manga", PageCount: 2, TotalBookmarks: 3, TotalView: 4, XRestrict: 1,
		User:           model.User{ID: 5, Name: "name", Account: "account", Comment: "comment", IsFollowed: true},
		Tags:           []model.Tag{{Name: "tag", TranslatedName: "translated"}},
		ImageURLs:      model.ImageURLs{SquareMedium: "square", Medium: "medium", Large: "large", Original: "original"},
		MetaSinglePage: model.SinglePage{OriginalImageURL: "single"},
		MetaPages:      []model.MetaPage{{PageIndex: 1, Width: 6, Height: 7, Extension: "png", ImageURLs: model.ImageURLs{Original: "page"}}},
		AIType:         2, CreateDate: "2026-07-12", Width: 8, Height: 9,
	}
	if got := mapIllust(dto); !reflect.DeepEqual(got, want) {
		t.Fatalf("mapIllust() = %#v, want %#v", got, want)
	}
}

func TestMapUserDetailPreservesNormalizedEnvelopeAndOptionalURLs(t *testing.T) {
	profileImageURL := "profile"
	webpage := "webpage"
	background := "background"
	twitter := "twitter"
	pawoo := "pawoo"
	workspaceImage := "workspace"
	dto := userDetailDTO{
		User: requiredObject[userDTO]{Present: true, Valid: true, Value: userDTO{
			ID: 1, Name: "name", ProfileImageURLs: profileImageURLsDTO{Medium: &profileImageURL},
		}},
		Profile: requiredObject[profileDTO]{Present: true, Valid: true, Value: profileDTO{
			Webpage: &webpage, Gender: "gender", Birth: "birth", BirthDay: "day", BirthYear: 2000, Region: "region",
			AddressID: 2, CountryCode: "JP", Job: "job", JobID: 3, TotalFollowUsers: 4, TotalMyPixivUsers: 5,
			TotalIllusts: 6, TotalManga: 7, TotalNovels: 8, TotalIllustBookmarksPublic: 9, TotalIllustSeries: 10,
			TotalNovelSeries: 11, BackgroundImageURL: &background, TwitterAccount: "account", TwitterURL: &twitter,
			PawooURL: &pawoo, IsPremium: true, IsUsingCustomProfileImage: true,
		}},
		ProfilePublicity: requiredObject[profilePublicityDTO]{Present: true, Valid: true, Value: profilePublicityDTO{
			Gender: true, Region: true, BirthDay: true, BirthYear: true, Job: true, Pawoo: true,
		}},
		Workspace: requiredObject[workspaceDTO]{Present: true, Valid: true, Value: workspaceDTO{
			PC: "pc", Monitor: "monitor", Tool: "tool", Scanner: "scanner", Tablet: "tablet", Mouse: "mouse",
			Printer: "printer", Desktop: "desktop", Music: "music", Desk: "desk", Chair: "chair", Comment: "comment",
			WorkspaceImageURL: &workspaceImage,
		}},
	}
	want := model.UserDetail{
		User: model.User{ID: 1, Name: "name", ProfileImageURLs: model.ProfileImageURLs{Medium: &profileImageURL}},
		Profile: model.Profile{
			Webpage: &webpage, Gender: "gender", Birth: "birth", BirthDay: "day", BirthYear: 2000, Region: "region",
			AddressID: 2, CountryCode: "JP", Job: "job", JobID: 3, TotalFollowUsers: 4, TotalMyPixivUsers: 5,
			TotalIllusts: 6, TotalManga: 7, TotalNovels: 8, TotalIllustBookmarksPublic: 9, TotalIllustSeries: 10,
			TotalNovelSeries: 11, BackgroundImageURL: &background, TwitterAccount: "account", TwitterURL: &twitter,
			PawooURL: &pawoo, IsPremium: true, IsUsingCustomProfileImage: true,
		},
		ProfilePublicity: model.ProfilePublicity{Gender: true, Region: true, BirthDay: true, BirthYear: true, Job: true, Pawoo: true},
		Workspace: model.Workspace{
			PC: "pc", Monitor: "monitor", Tool: "tool", Scanner: "scanner", Tablet: "tablet", Mouse: "mouse",
			Printer: "printer", Desktop: "desktop", Music: "music", Desk: "desk", Chair: "chair", Comment: "comment",
			WorkspaceImageURL: &workspaceImage,
		},
	}
	if got := mapUserDetail(dto); !reflect.DeepEqual(got, want) {
		t.Fatalf("mapUserDetail() = %#v, want %#v", got, want)
	}
}
