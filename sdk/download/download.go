// Package download saves selected Pixiv artwork pages with filename templates.
//
// [New] builds a [Downloader] from [Options]. [Downloader.Save] accepts pages
// from several artworks and returns a [Report] containing files and diagnostics.
package download

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/media/filename"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
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
	var report Report
	if err := ctx.Err(); err != nil {
		return report, err
	}
	options := d.options
	if options.Quality == "" {
		options.Quality = QualityOriginal
	}
	if options.UgoiraFormat == "" {
		options.UgoiraFormat = UgoiraFormatGIF
	}
	switch options.Quality {
	case QualityOriginal, QualityRegular, QualitySmall, QualityThumb, QualityMini:
	default:
		return report, invalidArgument("quality must be one of original, regular, small, thumb, mini")
	}
	switch options.UgoiraFormat {
	case UgoiraFormatGIF, UgoiraFormatAPNG:
	default:
		return report, invalidArgument("ugoira format must be one of gif, apng")
	}
	options.DirectoryTemplate = strings.TrimSpace(options.DirectoryTemplate)
	if err := filename.ValidateDirectoryTemplate(options.DirectoryTemplate); err != nil {
		return report, invalidArgument(err.Error())
	}
	base, err := filepath.Abs(options.DownloadPath)
	if err != nil {
		return report, err
	}
	var failures []error
	for _, page := range pages {
		if err := ctx.Err(); err != nil {
			return report, errors.Join(append(failures, err)...)
		}
		file := File{PageIndex: page.PageIndex}
		if page.Artwork != nil && page.Artwork.ArtworkRef != nil {
			file.ArtworkID = page.Artwork.ID
		}
		output, warnings, err := savePage(ctx, options, base, page)
		report.Warnings = append(report.Warnings, warnings...)
		if output != "" {
			file.Path = output
			report.Files = append(report.Files, file)
		}
		if err != nil {
			if sdk.ReasonOf(err) == "" {
				reason := sdk.LocalStateError
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					reason = sdk.UpstreamUnavailable
				}
				err = &sdk.Error{Operation: "download.Save", Reason: reason, Message: "save page failed", Err: err}
			}
			failure := fmt.Errorf("artwork %d page %d: %w", file.ArtworkID, page.PageIndex, err)
			report.Failures = append(report.Failures, Failure{ArtworkID: file.ArtworkID, PageIndex: page.PageIndex, Cause: failure})
			failures = append(failures, failure)
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return report, errors.Join(failures...)
			}
		}
	}
	return report, errors.Join(append(failures, ctx.Err())...)
}

func invalidArgument(message string) *sdk.Error {
	return &sdk.Error{Operation: "download.Save", Reason: sdk.InvalidArgument, Message: message}
}
