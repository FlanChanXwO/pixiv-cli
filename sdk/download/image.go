package download

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/media/filename"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/atomicfile"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

const imageSniffBytes = 512 // http.DetectContentType examines at most this many bytes.

func imageForQuality(image pixiv.Image, quality Quality) (pixiv.Image, error) {
	if quality == QualityOriginal {
		return image, nil
	}
	original, err := url.Parse(image.URL)
	if err != nil {
		return pixiv.Image{}, invalidArgument("invalid original image URL")
	}
	const originalMarker = "/img-original/img/"
	_, relative, ok := strings.Cut(original.Path, originalMarker)
	if !ok || original.Scheme != "https" {
		return pixiv.Image{}, invalidArgument("image quality variant is unavailable")
	}
	var crop, suffix string
	switch quality {
	case QualityRegular:
		crop, suffix = "1200x1200", "_master1200"
	case QualitySmall:
		crop, suffix = "540x540_70", "_master1200"
	case QualityThumb:
		crop, suffix = "250x250_80_a2", "_square1200"
	case QualityMini:
		crop, suffix = "48x48", "_square1200"
	}
	// Keep the account binding while applying the same CDN convention as the Pixiv client.
	original.Path = "/c/" + crop + "/img-master/img/" + strings.TrimSuffix(relative, path.Ext(relative)) + suffix + ".jpg"
	original.RawPath, original.RawQuery, original.Fragment = "", "", ""
	image.URL = original.String()
	return image, nil
}

func saveImage(ctx context.Context, image pixiv.Image, base, name string) (output string, returnErr error) {
	body, err := image.Open(ctx)
	if err != nil {
		return "", err
	}
	defer func() { returnErr = errors.Join(returnErr, body.Close()) }()
	reader := bufio.NewReader(body)
	header, err := reader.Peek(imageSniffBytes)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	locator, err := url.Parse(image.URL)
	if err != nil {
		return "", invalidArgument("invalid image URL")
	}
	var extension string
	switch http.DetectContentType(header) {
	case "image/jpeg":
		extension = ".jpg"
		if strings.EqualFold(path.Ext(locator.Path), ".jpeg") {
			extension = path.Ext(locator.Path)
		}
	case "image/png":
		extension = ".png"
	case "image/gif":
		extension = ".gif"
	case "image/webp":
		extension = ".webp"
	default:
		// Image.Open exposes bytes without headers; retain the old URL-extension fallback.
		extension = filename.Sanitize(path.Ext(locator.Path))
		extension = strings.TrimRight(strings.Map(func(r rune) rune {
			if r < 0x20 || r == 0x7f {
				return '_'
			}
			return r
		}, extension), ". ")
	}
	output = filepath.Join(base, name+extension)
	if _, err := atomicfile.AtomicWrite(ctx, output, reader); err != nil {
		return "", err
	}
	return output, nil
}
