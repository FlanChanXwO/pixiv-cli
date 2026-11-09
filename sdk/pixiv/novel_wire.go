package pixiv

import (
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// The pointer fields are mandatory in search responses and optional elsewhere.
type novelWire struct {
	ID             int64         `json:"id"`
	Title          string        `json:"title"`
	Caption        string        `json:"caption"`
	XRestrict      *int          `json:"x_restrict"`
	TextLength     *int          `json:"text_length"`
	IsOriginal     *bool         `json:"is_original"`
	User           userWire      `json:"user"`
	Tags           []tagWire     `json:"tags"`
	ImageURLs      imageURLsWire `json:"image_urls"`
	CreateDate     string        `json:"create_date"`
	TotalBookmarks int           `json:"total_bookmarks"`
	TotalView      int           `json:"total_view"`
}

type novelListWire struct {
	Novels  api.RequiredList[novelWire] `json:"novels"`
	NextURL *string                     `json:"next_url"`
}

func (c *Client) mapNovel(w novelWire) (*Novel, error) {
	if w.ID <= 0 || w.User.ID <= 0 {
		return nil, newError("Novel", sdk.MalformedUpstreamResponse, "novel and author IDs must be positive", nil)
	}
	published, err := time.Parse(time.RFC3339, w.CreateDate)
	if err != nil {
		return nil, newError("Novel", sdk.MalformedUpstreamResponse, "invalid publish time", err)
	}
	author, err := c.mapUser(w.User)
	if err != nil {
		return nil, err
	}
	cover, err := c.image(firstImageURL(w.ImageURLs), 0, 0)
	if err != nil {
		return nil, err
	}
	n := &Novel{NovelRef: c.Novel(w.ID), novelData: novelData{
		Title: w.Title, Caption: w.Caption, Author: author, Cover: cover, Tags: make([]Tag, len(w.Tags)), PublishedAt: published.UTC(), TotalBookmarks: w.TotalBookmarks, TotalViews: w.TotalView,
	}}
	if w.XRestrict != nil {
		n.XRestrict = *w.XRestrict
	}
	if w.TextLength != nil {
		n.TextLength = *w.TextLength
	}
	if w.IsOriginal != nil {
		n.IsOriginal = *w.IsOriginal
	}
	for i, tag := range w.Tags {
		n.Tags[i] = Tag{Name: tag.Name, TranslatedName: tag.TranslatedName}
	}
	return n, nil
}
