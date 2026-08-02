package pixiv

import (
	"errors"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/model"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

var errNoPublishTime = errors.New("no publish time")

// mapArtwork converts a normalized adapter artwork into the public model. The
// cover is the largest available size; pages are populated from detail results.
func (c *Client) mapArtwork(m model.Illust) (Artwork, error) {
	published, err := parseUTCTime(m.CreateDate)
	if err != nil {
		return Artwork{}, newError("Artwork", sdk.CodeMalformedUpstreamResponse, "invalid publish time")
	}
	kind, raw := artworkKind(m.Type)
	out := Artwork{
		ID:             m.ID,
		Title:          m.Title,
		Caption:        m.Caption,
		Kind:           kind,
		RawKind:        raw,
		Tags:           mapTags(m.Tags),
		User:           c.mapUser(m.User),
		PublishedAt:    published,
		TotalBookmarks: m.TotalBookmarks,
		TotalViews:     m.TotalView,
		Width:          m.Width,
		Height:         m.Height,
		PageCount:      m.PageCount,
		XRestrict:      m.XRestrict,
		AIType:         m.AIType,
		Tools:          m.Tools,
	}
	out.Cover, err = c.mapArtworkCover(m)
	if err != nil {
		return Artwork{}, err
	}
	return out, nil
}

// mapArtworkDetail additionally populates pages from a detail result.
func (c *Client) mapArtworkDetail(m model.Illust) (Artwork, error) {
	out, err := c.mapArtwork(m)
	if err != nil {
		return Artwork{}, err
	}
	pages, err := c.mapArtworkPages(m)
	if err != nil {
		return Artwork{}, err
	}
	out.Pages = pages
	return out, nil
}

func (c *Client) mapArtworkCover(m model.Illust) (ImageResource, error) {
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
			res, err := c.newResource("artwork", m.ID, -1, size.value)
			if err != nil {
				return ImageResource{}, err
			}
			return ImageResource{Resource: res, Variant: size.variant, Width: m.Width, Height: m.Height}, nil
		}
	}
	return ImageResource{}, nil
}

func (c *Client) mapArtworkPages(m model.Illust) ([]ArtworkPage, error) {
	if len(m.MetaPages) > 0 {
		pages := make([]ArtworkPage, 0, len(m.MetaPages))
		for _, page := range m.MetaPages {
			url := firstAvailable(page.ImageURLs)
			if url == "" {
				return nil, newError("ArtworkPages", sdk.CodeMalformedUpstreamResponse, "page has no image URL")
			}
			res, err := c.newResource("artwork", m.ID, page.PageIndex, url)
			if err != nil {
				return nil, err
			}
			pages = append(pages, ArtworkPage{
				PageIndex: page.PageIndex,
				Image:     ImageResource{Resource: res, Variant: "original", Width: page.Width, Height: page.Height},
				Width:     page.Width,
				Height:    page.Height,
			})
		}
		return pages, nil
	}
	if m.MetaSinglePage.OriginalImageURL != "" {
		res, err := c.newResource("artwork", m.ID, 0, m.MetaSinglePage.OriginalImageURL)
		if err != nil {
			return nil, err
		}
		return []ArtworkPage{{PageIndex: 0, Image: ImageResource{Resource: res, Variant: "original", Width: m.Width, Height: m.Height}, Width: m.Width, Height: m.Height}}, nil
	}
	return nil, nil
}

func (c *Client) mapNovel(m model.Novel) (Novel, error) {
	published, err := parseUTCTime(m.CreateDate)
	if err != nil {
		return Novel{}, newError("Novel", sdk.CodeMalformedUpstreamResponse, "invalid publish time")
	}
	out := Novel{
		ID:             m.ID,
		Title:          m.Title,
		Caption:        m.Caption,
		User:           c.mapUser(m.User),
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
			res, err := c.newResource("novel_cover", m.ID, -1, size.value)
			if err != nil {
				return Novel{}, err
			}
			out.Cover = ImageResource{Resource: res, Variant: size.variant}
			break
		}
	}
	return out, nil
}

func (c *Client) mapUser(m model.User) User {
	out := User{
		ID:         m.ID,
		Name:       m.Name,
		Account:    m.Account,
		Comment:    m.Comment,
		IsFollowed: m.IsFollowed,
	}
	if m.ProfileImageURLs.Medium != nil && *m.ProfileImageURLs.Medium != "" {
		if res, err := c.newResource("user_profile", m.ID, -1, *m.ProfileImageURLs.Medium); err == nil {
			out.ProfileImage = ImageResource{Resource: res, Variant: "medium"}
		}
	}
	return out
}

func (c *Client) mapUserDetail(d model.UserDetail) (UserDetail, error) {
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

func (c *Client) mapUserPreview(p model.RecommendedUserPreview) UserPreview {
	out := UserPreview{User: c.mapUser(p.User)}
	if len(p.Illusts) > 0 {
		out.Illusts = make([]Artwork, 0, len(p.Illusts))
		for _, m := range p.Illusts {
			if a, err := c.mapArtwork(m); err == nil {
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

func mapTags(tags []model.Tag) []Tag {
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

func firstAvailable(urls model.ImageURLs) string {
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
