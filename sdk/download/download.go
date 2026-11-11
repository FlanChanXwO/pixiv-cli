// Package download saves selected Pixiv artwork pages with filename templates.
//
// [New] builds a [Downloader] from [Options]. [Downloader.Save] accepts pages
// from several artworks and returns a [Report] containing files and diagnostics.
package download

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// Quality selects a static image resolution; the empty value selects original.
type Quality string

const (
	// QualityOriginal saves the original image.
	QualityOriginal Quality = "original"
	// QualityRegular saves the regular image.
	QualityRegular Quality = "regular"
	// QualitySmall saves the small image.
	QualitySmall Quality = "small"
	// QualityThumb saves the thumbnail.
	QualityThumb Quality = "thumb"
	// QualityMini saves the smallest thumbnail.
	QualityMini Quality = "mini"
)

// UgoiraFormat selects the animation container; the empty value selects GIF.
type UgoiraFormat string

const (
	// UgoiraFormatGIF encodes animated GIF files.
	UgoiraFormatGIF UgoiraFormat = "gif"
	// UgoiraFormatAPNG encodes animated PNG files with an .apng extension.
	UgoiraFormatAPNG UgoiraFormat = "apng"
)

// Options controls paths, templates and image output without reading config files.
// Templates accept {id}, {title}, {author}, {author_id}, {date}, {tags} and
// zero-based {num}. An empty filename template uses the default author/title/ID
// name. An empty directory template groups multipage and animated artworks by
// ID and title. Quality applies to static images; Ugoira requires original.
type Options struct {
	DownloadPath      string // Empty uses the current directory.
	FilenameTemplate  string
	DirectoryTemplate string // Slash-separated relative directories.
	Quality           Quality
	UgoiraFormat      UgoiraFormat
}

// Downloader retains its options; artwork pages carry their own account binding.
type Downloader struct{ options Options }

// New records options without filesystem or network I/O; Save validates them.
func New(options Options) *Downloader { return &Downloader{options: options} }

// Save processes only the supplied pages, in input order. A page failure is
// recorded and processing continues; the returned error joins the failures.
// Cancellation stops the batch and returns the context error with partial results.
func (d *Downloader) Save(ctx context.Context, pages ...pixiv.ArtworkPage) (Report, error) {
	return Report{}, errors.New("download saving is not implemented")
}
