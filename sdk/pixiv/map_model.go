package pixiv

import (
	"errors"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/novel"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/user"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

var errNoPublishTime = errors.New("no publish time")

func (c *Client) mapNovel(m novel.Novel) (Novel, error) {
	published, err := parseUTCTime(m.CreateDate)
	if err != nil {
		return Novel{}, newError("Novel", sdk.MalformedUpstreamResponse, "invalid publish time")
	}
	out := Novel{
		ID:             m.ID,
		Title:          m.Title,
		Caption:        m.Caption,
		User:           c.mapNovelUser(m.User),
		Tags:           mapTags(m.Tags),
		PublishedAt:    published,
		XRestrict:      m.XRestrict,
		TextLength:     m.TextLength,
		IsOriginal:     m.IsOriginal,
		TotalBookmarks: m.TotalBookmarks,
		TotalViews:     m.TotalView,
	}
	for _, size := range []struct {
		variant string
		value   string
	}{
		{"original", m.ImageURLs.Original},
		{"large", m.ImageURLs.Large},
		{"medium", m.ImageURLs.Medium},
		{"square_medium", m.ImageURLs.SquareMedium},
	} {
		if size.value != "" {
			res, err := c.newResourceWithVariant("novel_cover", m.ID, -1, size.variant, size.value)
			if err != nil {
				return Novel{}, err
			}
			out.Cover = ImageResource{Resource: res, Variant: size.variant}
			break
		}
	}
	return out, nil
}

func (c *Client) mapNovelUser(m novel.UserSummary) User {
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

func (c *Client) mapUser(m user.User) User {
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
	out := UserDetail{User: c.mapUser(d.User)}
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
	out := UserPreview{User: c.mapUser(p.User)}
	if len(p.Illusts) > 0 {
		out.Illusts = make([]Artwork, 0, len(p.Illusts))
		for _, m := range p.Illusts {
			if a, err := c.mapArtworkEntity(m); err == nil {
				out.Illusts = append(out.Illusts, a)
			}
		}
	}
	if len(p.Novels) > 0 {
		out.Novels = make([]Novel, 0, len(p.Novels))
		for _, m := range p.Novels {
			if n, err := c.mapNovel(m); err == nil {
				out.Novels = append(out.Novels, n)
			}
		}
	}
	return out
}

func mapTags(tags []novel.Tag) []Tag {
	if len(tags) == 0 {
		return []Tag{}
	}
	out := make([]Tag, 0, len(tags))
	for _, t := range tags {
		out = append(out, Tag{Name: t.Name, TranslatedName: t.TranslatedName})
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

func firstAvailable(urls novel.ImageURLs) string {
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
