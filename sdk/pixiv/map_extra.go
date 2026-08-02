package pixiv

import (
	"net/url"
	"path"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/model"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// mapUgoiraMetadata converts the adapter ugoira result into the public model,
// validating frame filenames and archive quality. It returns
// CodeMalformedUpstreamResponse when the result is incomplete or unsafe.
func (c *Client) mapUgoiraMetadata(artworkID int64, result *model.UgoiraMetadataResult) (UgoiraMetadata, error) {
	meta := result.UgoiraMetadata
	if artworkID <= 0 || len(meta.Frames) == 0 {
		return UgoiraMetadata{}, newError("UgoiraMetadata", sdk.CodeMalformedUpstreamResponse, "incomplete ugoira metadata")
	}
	archives := make([]UgoiraArchive, 0, 2)
	seenQuality := map[string]bool{}
	for _, candidate := range []struct {
		quality UgoiraQuality
		url     string
	}{
		{UgoiraQualityMedium, meta.ZipURLs.Medium},
		{UgoiraQualityOriginal, meta.ZipURLs.Original},
	} {
		if candidate.url == "" || seenQuality[string(candidate.quality)] {
			continue
		}
		res, err := c.newResource("ugoira_archive", artworkID, -1, candidate.url)
		if err != nil {
			return UgoiraMetadata{}, err
		}
		archives = append(archives, UgoiraArchive{Quality: candidate.quality, Resource: res})
		seenQuality[string(candidate.quality)] = true
	}
	if len(archives) == 0 {
		return UgoiraMetadata{}, newError("UgoiraMetadata", sdk.CodeMalformedUpstreamResponse, "ugoira metadata has no archive")
	}
	frames := make([]UgoiraFrame, 0, len(meta.Frames))
	seenFile := map[string]bool{}
	for _, frame := range meta.Frames {
		if !safeArchiveFilename(frame.File) || seenFile[frame.File] {
			return UgoiraMetadata{}, newError("UgoiraMetadata", sdk.CodeMalformedUpstreamResponse, "ugoira frame filename is unsafe or duplicated")
		}
		seenFile[frame.File] = true
		frames = append(frames, UgoiraFrame{Filename: frame.File, DelayMilliseconds: frame.Delay})
	}
	if len(frames) == 0 {
		return UgoiraMetadata{}, newError("UgoiraMetadata", sdk.CodeMalformedUpstreamResponse, "ugoira metadata has no frames")
	}
	return UgoiraMetadata{ArtworkID: artworkID, Archives: archives, Frames: frames}, nil
}

func safeArchiveFilename(name string) bool {
	if name == "" || strings.ContainsAny(name, "\\") || path.IsAbs(name) {
		return false
	}
	cleaned := path.Clean(name)
	if cleaned != name || cleaned == "." || strings.HasPrefix(cleaned, "..") {
		return false
	}
	return true
}

// commentPage maps an adapter comment list into a public CommentPage, encoding
// the offset continuation into an opaque cursor.
func (c *Client) commentPage(op string, query url.Values, list *model.CommentList) (CommentPage, error) {
	items := make([]Comment, 0, len(list.Comments))
	for _, m := range list.Comments {
		comment, err := c.mapComment(m)
		if err != nil {
			return CommentPage{}, err
		}
		items = append(items, comment)
	}
	next, err := c.buildCursor(op, query, "offset", int64(list.NextOffset), list.ContinuationExists)
	if err != nil {
		return CommentPage{}, err
	}
	page := CommentPage{Page: sdk.Page[Comment]{Items: items, Next: next}}
	if list.Total != nil {
		value := *list.Total
		page.Total = &value
	}
	if list.AccessControl != nil {
		page.AccessControl = &CommentAccessControl{
			CanComment: list.AccessControl.CanComment,
			IsLocked:   list.AccessControl.IsLocked,
		}
	}
	return page, nil
}

func (c *Client) mapComment(m model.Comment) (Comment, error) {
	created, err := parseUTCTime(m.CreateDate)
	if err != nil {
		return Comment{}, newError("Comment", sdk.CodeMalformedUpstreamResponse, "invalid comment time")
	}
	out := Comment{ID: m.ID, User: c.mapUser(m.User), Comment: m.Comment, CreatedAt: created}
	if m.ParentComment != nil {
		parent, err := c.mapComment(*m.ParentComment)
		if err != nil {
			return Comment{}, err
		}
		out.ParentComment = &parent
	}
	return out, nil
}
