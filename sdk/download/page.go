package download

import (
	"cmp"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/media/filename"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func savePage(ctx context.Context, options Options, base string, page pixiv.ArtworkPage) (string, []Warning, error) {
	art := page.Artwork
	if art == nil || art.ArtworkRef == nil || art.ID <= 0 {
		return "", nil, invalidArgument("page must have an owning artwork")
	}
	if page.PageIndex < 0 || page.PageIndex >= len(art.Pages) {
		return "", nil, invalidArgument("page index is outside the owning artwork")
	}
	animated := art.ContentType == pixiv.ContentTypeUgoira
	if animated && options.Quality != QualityOriginal {
		return "", nil, invalidArgument("ugoira only supports original quality")
	}
	fields := filename.Fields{ID: art.ID, Title: art.Title, PageCount: art.PageCount}
	if art.Author != nil {
		fields.Author = art.Author.Name
		if art.Author.UserRef != nil {
			fields.AuthorID = art.Author.ID
		}
	}
	if !art.PublishedAt.IsZero() {
		fields.CreateDate = art.PublishedAt.Format(time.RFC3339)
	}
	for _, tag := range art.Tags {
		if name := strings.TrimSpace(tag.Name); name != "" {
			fields.Tags = append(fields.Tags, name)
		}
	}
	directory, err := filename.BuildRelativeDirectory(options.DirectoryTemplate, fields, page.PageIndex)
	if err != nil {
		return "", nil, invalidArgument(err.Error())
	}
	if options.DirectoryTemplate == "" && (art.PageCount > 1 || animated) {
		directory = filename.Sanitize(fmt.Sprintf("%d - %s", art.ID, cmp.Or(art.Title, "Untitled")))
	}
	base = filepath.Join(base, filepath.FromSlash(directory))
	name, err := filename.GenerateChecked(fields, page.PageIndex, options.FilenameTemplate)
	var warnings []Warning
	if animated && (err != nil || name == "") {
		message := "ugoira filename template failed; using default filename"
		if err == nil {
			message = "ugoira filename template produced an empty name; using default filename"
		}
		warnings = append(warnings, Warning{ArtworkID: art.ID, PageIndex: page.PageIndex, Message: message, Cause: err})
		name, err = filename.GenerateChecked(fields, page.PageIndex, "")
	}
	if err != nil {
		return "", warnings, invalidArgument(err.Error())
	}
	if name == "" || name == "." || name == ".." {
		return "", warnings, invalidArgument("filename template produced an unsafe filename")
	}
	if animated {
		path, err := saveUgoira(ctx, art, base, name, options.UgoiraFormat)
		return path, warnings, err
	}
	image, err := page.Original.Quality(string(options.Quality))
	if err != nil {
		return "", warnings, err
	}
	path, err := saveImage(ctx, image, base, name)
	return path, warnings, err
}
