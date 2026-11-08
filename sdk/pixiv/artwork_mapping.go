package pixiv

import (
	"cmp"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

func (c *Client) mapArtwork(w artworkWire) (*Artwork, error) {
	if w.ID <= 0 {
		return nil, newError("Artwork", sdk.MalformedUpstreamResponse, "missing artwork ID", nil)
	}
	published, err := time.Parse(time.RFC3339, w.CreateDate)
	if err != nil {
		return nil, newError("Artwork", sdk.MalformedUpstreamResponse, "invalid publish time", err)
	}
	author, err := c.mapUser(w.User)
	if err != nil {
		return nil, err
	}
	a := &Artwork{ArtworkRef: c.Artwork(w.ID), artworkData: artworkData{
		Title: w.Title, Caption: w.Caption, ContentType: ContentType(w.Type), Author: author, Tags: make([]Tag, len(w.Tags)), PublishedAt: published.UTC(),
		TotalBookmarks: w.TotalBookmarks, TotalViews: w.TotalView, Width: w.Width, Height: w.Height, PageCount: w.PageCount, XRestrict: w.XRestrict, AIType: w.AIType,
		Tools: w.Tools, Pages: []ArtworkPage{}, IsBookmarked: w.IsBookmarked, IsMuted: w.IsMuted, Visible: w.Visible, SanityLevel: w.SanityLevel,
		RestrictionAttributes: w.RestrictionAttributes, TotalComments: w.TotalComments,
	}}
	if a.Tools == nil {
		a.Tools = []string{}
	}
	for i, tag := range w.Tags {
		a.Tags[i] = Tag{Name: tag.Name, TranslatedName: tag.TranslatedName}
	}
	if w.Series != nil {
		a.Series = &ArtworkSeriesSummary{ID: w.Series.ID, Title: w.Series.Title}
	}
	a.Cover, err = c.image(firstImageURL(w.ImageURLs), w.Width, w.Height)
	if err != nil {
		return nil, err
	}
	for index, page := range w.MetaPages {
		raw := firstImageURL(page.ImageURLs)
		if raw == "" {
			return nil, newError("Artwork", sdk.MalformedUpstreamResponse, "page has no image URL", nil)
		}
		image, err := c.image(raw, page.Width, page.Height)
		if err != nil {
			return nil, err
		}
		a.Pages = append(a.Pages, ArtworkPage{Artwork: a, PageIndex: index, Original: image, Width: page.Width, Height: page.Height})
	}
	if a.ContentType == ContentTypeUgoira {
		a.Pages = []ArtworkPage{{Artwork: a, PageIndex: 0, Original: a.Cover, Width: w.Width, Height: w.Height}}
	} else if len(a.Pages) == 0 && w.MetaSinglePage.OriginalImageURL != "" {
		image, err := c.image(w.MetaSinglePage.OriginalImageURL, w.Width, w.Height)
		if err != nil {
			return nil, err
		}
		a.Pages = append(a.Pages, ArtworkPage{Artwork: a, PageIndex: 0, Original: image, Width: w.Width, Height: w.Height})
	}
	return a, nil
}

func firstImageURL(w imageURLsWire) string {
	return cmp.Or(w.Original, w.Large, w.Medium, w.SquareMedium)
}

func (c *Client) mapUser(w userWire) (*User, error) {
	u := &User{UserRef: c.User(w.ID), Name: w.Name, Account: w.Account, Comment: w.Comment, IsFollowed: w.IsFollowed}
	if w.ProfileImageURLs.Medium != nil {
		image, err := c.image(*w.ProfileImageURLs.Medium, 0, 0)
		if err != nil {
			return nil, err
		}
		u.ProfileImage = image
	}
	return u, nil
}
