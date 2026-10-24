package pixiv

import (
	"errors"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/novel"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user"
)

var errNoPublishTime = errors.New("no publish time")

// mapNovelEntity 把 internal novel 实体（user 小说端点仍在读取）转成公开模型；
// wire 映射与资源注册由 wire.go 的 mapNovel 统一完成。
func (c *Client) mapNovelEntity(m novel.Novel) (Novel, error) {
	return c.mapNovel(novelWireFromEntity(m))
}

// mapUserEntity 映射 user 端点实体；user 端点并入 SDK 后由 wire.go 的 mapUser
// 取代（G08）。
func (c *Client) mapUserEntity(m user.User) User {
	out := User{
		ID:         m.ID,
		Name:       m.Name,
		Account:    m.Account,
		Comment:    m.Comment,
		IsFollowed: m.IsFollowed,
	}
	if m.ProfileImageURLs.Medium != nil && *m.ProfileImageURLs.Medium != "" {
		if res, err := c.newResourceWithVariant("user_profile", m.ID, -1, "medium", *m.ProfileImageURLs.Medium); err == nil {
			out.ProfileImage = ImageResource{Resource: res, Variant: "medium"}
		}
	}
	return out
}

func (c *Client) mapUserDetail(d user.Detail) (UserDetail, error) {
	out := UserDetail{User: c.mapUserEntity(d.User)}
	out.Profile = UserProfile{
		Webpage:                   strValue(d.Profile.Webpage),
		Gender:                    d.Profile.Gender,
		BirthDay:                  d.Profile.BirthDay,
		BirthYear:                 d.Profile.BirthYear,
		Region:                    d.Profile.Region,
		CountryCode:               d.Profile.CountryCode,
		Job:                       d.Profile.Job,
		TotalFollowUsers:          d.Profile.TotalFollowUsers,
		TotalMyPixivUsers:         d.Profile.TotalMyPixivUsers,
		TotalIllusts:              d.Profile.TotalIllusts,
		TotalManga:                d.Profile.TotalManga,
		TotalNovels:               d.Profile.TotalNovels,
		TotalIllustBookmarks:      d.Profile.TotalIllustBookmarksPublic,
		TotalIllustSeries:         d.Profile.TotalIllustSeries,
		TotalNovelSeries:          d.Profile.TotalNovelSeries,
		BackgroundImageURL:        strValue(d.Profile.BackgroundImageURL),
		TwitterAccount:            d.Profile.TwitterAccount,
		TwitterURL:                strValue(d.Profile.TwitterURL),
		PawooURL:                  strValue(d.Profile.PawooURL),
		IsPremium:                 d.Profile.IsPremium,
		IsUsingCustomProfileImage: d.Profile.IsUsingCustomProfileImage,
	}
	out.ProfilePublicity = UserProfilePublicity{
		Gender:    d.ProfilePublicity.Gender,
		Region:    d.ProfilePublicity.Region,
		BirthDay:  d.ProfilePublicity.BirthDay,
		BirthYear: d.ProfilePublicity.BirthYear,
		Job:       d.ProfilePublicity.Job,
		Pawoo:     d.ProfilePublicity.Pawoo,
	}
	out.Workspace = UserWorkspace{
		PC:                d.Workspace.PC,
		Monitor:           d.Workspace.Monitor,
		Tool:              d.Workspace.Tool,
		Scanner:           d.Workspace.Scanner,
		Tablet:            d.Workspace.Tablet,
		Mouse:             d.Workspace.Mouse,
		Printer:           d.Workspace.Printer,
		Desktop:           d.Workspace.Desktop,
		Music:             d.Workspace.Music,
		Desk:              d.Workspace.Desk,
		Chair:             d.Workspace.Chair,
		Comment:           d.Workspace.Comment,
		WorkspaceImageURL: strValue(d.Workspace.WorkspaceImageURL),
	}
	return out, nil
}

func (c *Client) mapUserPreview(p user.Preview) UserPreview {
	out := UserPreview{User: c.mapUserEntity(p.User), Illusts: make([]Artwork, 0, len(p.Illusts)), Novels: make([]Novel, 0, len(p.Novels))}
	for _, m := range p.Illusts {
		if a, err := c.mapArtworkEntity(m); err == nil {
			out.Illusts = append(out.Illusts, a)
		}
	}
	for _, m := range p.Novels {
		if n, err := c.mapNovelEntity(m); err == nil {
			out.Novels = append(out.Novels, n)
		}
	}
	return out
}

func artworkKind(raw string) (ArtworkKind, string) {
	switch raw {
	case "illust":
		return ArtworkKindIllustration, raw
	case "manga":
		return ArtworkKindManga, raw
	case "ugoira":
		return ArtworkKindUgoira, raw
	default:
		return ArtworkKindUnknown, raw
	}
}

func parseUTCTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, errNoPublishTime
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}

func firstAvailable(urls imageURLsWire) string {
	for _, value := range []string{urls.Original, urls.Large, urls.Medium, urls.SquareMedium} {
		if value != "" {
			return value
		}
	}
	return ""
}

func strValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
