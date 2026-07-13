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
