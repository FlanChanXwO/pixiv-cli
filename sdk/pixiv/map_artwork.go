package pixiv

import (
	"net/url"
	"path"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/artwork"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// mapArtworkEntity 将 endpoint family 的 normalized artwork 转为 public
// runtime model，并在此边界创建受 policy 约束的 Resource。
func (c *Client) mapArtworkEntity(value artwork.Artwork) (Artwork, error) {
	published, err := parseUTCTime(value.CreateDate)
	if err != nil {
		return Artwork{}, newError("Artwork", sdk.MalformedUpstreamResponse, "invalid publish time")
	}
	kind, raw := artworkKind(value.Type)
	result := Artwork{
		ID:             value.ID,
		Title:          value.Title,
		Caption:        value.Caption,
		Kind:           kind,
		RawKind:        raw,
		Tags:           mapArtworkTags(value.Tags),
		User:           c.mapArtworkUser(value.User),
		PublishedAt:    published,
		TotalBookmarks: value.TotalBookmarks,
		TotalViews:     value.TotalView,
		Width:          value.Width,
		Height:         value.Height,
		PageCount:      value.PageCount,
		XRestrict:      value.XRestrict,
		AIType:         value.AIType,
		Tools:          append([]string(nil), value.Tools...),
	}
	result.Cover, err = c.mapArtworkEntityCover(value)
	if err != nil {
		return Artwork{}, err
	}
	return result, nil
}

func (c *Client) mapArtworkEntityCover(value artwork.Artwork) (ImageResource, error) {
	for _, candidate := range []struct {
		variant string
		value   string
	}{
		{variant: "original", value: value.ImageURLs.Original},
		{variant: "large", value: value.ImageURLs.Large},
		{variant: "medium", value: value.ImageURLs.Medium},
		{variant: "square_medium", value: value.ImageURLs.SquareMedium},
	} {
		if candidate.value == "" {
			continue
		}
		resource, err := c.newResourceWithVariant("artwork", value.ID, -1, candidate.variant, candidate.value)
		if err != nil {
			return ImageResource{}, err
		}
		return ImageResource{Resource: resource, Variant: candidate.variant, Width: value.Width, Height: value.Height}, nil
	}
	return ImageResource{}, nil
}

func (c *Client) mapArtworkEntityDetail(value artwork.Artwork) (Artwork, error) {
	result, err := c.mapArtworkEntity(value)
	if err != nil {
		return Artwork{}, err
	}
	result.Pages, err = c.mapArtworkEntityPages(value)
	if err != nil {
		return Artwork{}, err
	}
	return result, nil
}

func (c *Client) mapArtworkEntityPages(value artwork.Artwork) ([]ArtworkPage, error) {
	if len(value.MetaPages) > 0 {
		pages := make([]ArtworkPage, 0, len(value.MetaPages))
		for _, page := range value.MetaPages {
			imageURL := firstArtworkImageURL(page.ImageURLs)
			if imageURL == "" {
				return nil, newError("ArtworkPages", sdk.MalformedUpstreamResponse, "page has no image URL")
			}
			resource, err := c.newResourceWithVariant("artwork", value.ID, page.PageIndex, "original", imageURL)
			if err != nil {
				return nil, err
			}
			pages = append(pages, ArtworkPage{
				PageIndex: page.PageIndex,
				Image: ImageResource{
					Resource: resource,
					Variant:  "original",
					Width:    page.Width,
					Height:   page.Height,
				},
				Width:  page.Width,
				Height: page.Height,
			})
		}
		return pages, nil
	}
	if value.MetaSinglePage.OriginalImageURL == "" {
		return nil, nil
	}
	resource, err := c.newResourceWithVariant("artwork", value.ID, 0, "original", value.MetaSinglePage.OriginalImageURL)
	if err != nil {
		return nil, err
	}
	return []ArtworkPage{{
		PageIndex: 0,
		Image: ImageResource{
			Resource: resource,
			Variant:  "original",
			Width:    value.Width,
			Height:   value.Height,
		},
		Width:  value.Width,
		Height: value.Height,
	}}, nil
}

func firstArtworkImageURL(value artwork.ImageURLs) string {
	for _, candidate := range []string{value.Original, value.Large, value.Medium, value.SquareMedium} {
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

func (c *Client) mapUgoiraMetadataEntity(artworkID int64, value artwork.UgoiraMetadata) (UgoiraMetadata, error) {
	if artworkID <= 0 || len(value.Frames) == 0 {
		return UgoiraMetadata{}, newError("UgoiraMetadata", sdk.MalformedUpstreamResponse, "incomplete ugoira metadata")
	}
	archives := make([]UgoiraArchive, 0, 2)
	seenQuality := map[string]bool{}
	for _, candidate := range []struct {
		quality UgoiraQuality
		url     string
	}{
		{UgoiraQualityMedium, value.ZipURLs.Medium},
		{UgoiraQualityOriginal, value.ZipURLs.Original},
	} {
		if candidate.url == "" || seenQuality[string(candidate.quality)] {
			continue
		}
		resource, err := c.newResourceWithVariant("ugoira_archive", artworkID, -1, string(candidate.quality), candidate.url)
		if err != nil {
			return UgoiraMetadata{}, err
		}
		archives = append(archives, UgoiraArchive{Quality: candidate.quality, Resource: resource})
		seenQuality[string(candidate.quality)] = true
	}
	if len(archives) == 0 {
		return UgoiraMetadata{}, newError("UgoiraMetadata", sdk.MalformedUpstreamResponse, "ugoira metadata has no archive")
	}
	frames := make([]UgoiraFrame, 0, len(value.Frames))
	seenFile := map[string]bool{}
	for _, frame := range value.Frames {
		if !safeArtworkArchiveFilename(frame.File) || seenFile[frame.File] {
			return UgoiraMetadata{}, newError("UgoiraMetadata", sdk.MalformedUpstreamResponse, "ugoira frame filename is unsafe or duplicated")
		}
		seenFile[frame.File] = true
		frames = append(frames, UgoiraFrame{Filename: frame.File, DelayMilliseconds: frame.Delay})
	}
	return UgoiraMetadata{ArtworkID: artworkID, Archives: archives, Frames: frames}, nil
}

func safeArtworkArchiveFilename(name string) bool {
	if name == "" || strings.ContainsAny(name, "\\") || path.IsAbs(name) {
		return false
	}
	cleaned := path.Clean(name)
	return cleaned == name && cleaned != "." && !strings.HasPrefix(cleaned, "..")
}

func (c *Client) mapArtworkUser(value artwork.UserSummary) User {
	result := User{
		ID:         value.ID,
		Name:       value.Name,
		Account:    value.Account,
		Comment:    value.Comment,
		IsFollowed: value.IsFollowed,
	}
	if value.ProfileImageURLs.Medium != nil && *value.ProfileImageURLs.Medium != "" {
		if resource, err := c.newResourceWithVariant("user_profile", value.ID, -1, "medium", *value.ProfileImageURLs.Medium); err == nil {
			result.ProfileImage = ImageResource{Resource: resource, Variant: "medium"}
		}
	}
	return result
}

func mapArtworkTags(values []artwork.Tag) []Tag {
	if values == nil {
		return nil
	}
	result := make([]Tag, len(values))
	for index, value := range values {
		result[index] = Tag{Name: value.Name, TranslatedName: value.TranslatedName}
	}
	return result
}

func (c *Client) artworkPage(op string, query url.Values, key string, values []artwork.Artwork, nextValue int64, hasNext bool) (sdk.Page[Artwork], error) {
	items := make([]Artwork, len(values))
	for index, value := range values {
		mapped, err := c.mapArtworkEntity(value)
		if err != nil {
			return sdk.Page[Artwork]{}, err
		}
		items[index] = mapped
	}
	next, err := c.buildCursor(op, query, key, int64(nextValue), hasNext)
	if err != nil {
		return sdk.Page[Artwork]{}, err
	}
	return sdk.Page[Artwork]{Items: items, Next: next}, nil
}

func (c *Client) mapArtworkCommentPage(op string, query url.Values, values []artwork.Comment, nextOffset int, hasNext bool, total *int64, access *artwork.CommentAccessControl) (CommentPage, error) {
	items := make([]Comment, len(values))
	for index, value := range values {
		mapped, err := c.mapArtworkComment(value)
		if err != nil {
			return CommentPage{}, err
		}
		items[index] = mapped
	}
	next, err := c.buildCursor(op, query, "offset", int64(nextOffset), hasNext)
	if err != nil {
		return CommentPage{}, err
	}
	page := CommentPage{Page: sdk.Page[Comment]{Items: items, Next: next}}
	if total != nil {
		value := *total
		page.Total = &value
	}
	if access != nil {
		page.AccessControl = &CommentAccessControl{CanComment: access.CanComment, IsLocked: access.IsLocked}
	}
	return page, nil
}

func (c *Client) mapArtworkComment(value artwork.Comment) (Comment, error) {
	created, err := parseUTCTime(value.CreateDate)
	if err != nil {
		return Comment{}, newError("Comment", sdk.MalformedUpstreamResponse, "invalid comment time")
	}
	result := Comment{ID: value.ID, User: c.mapArtworkUser(value.User), Comment: value.Comment, CreatedAt: created}
	if value.ParentComment != nil {
		parent, err := c.mapArtworkComment(*value.ParentComment)
		if err != nil {
			return Comment{}, err
		}
		result.ParentComment = &parent
	}
	return result, nil
}
