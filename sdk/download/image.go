package download

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/media/filename"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/atomicfile"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

const imageSniffBytes = 512 // http.DetectContentType examines at most this many bytes.
const imageDirectoryPattern = ".pixiv-image-*"
const imageFilename = "image"

func saveImage(ctx context.Context, image pixiv.Image, quality Quality, base, name string) (output string, returnErr error) {
	locator, err := url.Parse(image.URL)
	if err != nil {
		return "", invalidArgument("invalid image URL")
	}
	image, err = image.Quality(pixiv.ImageQuality(quality))
	if err != nil {
		return "", err
	}
	response, err := image.OpenResponse(ctx)
	if err != nil {
		return "", err
	}
	defer func() { returnErr = errors.Join(returnErr, response.Body.Close()) }()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", &sdk.Error{Operation: "download.Save", Reason: sdk.UpstreamError, HTTPStatus: response.StatusCode, Message: "resource returned a non-success status"}
	}
	reader := bufio.NewReader(response.Body)
	header, err := reader.Peek(imageSniffBytes)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	suffix := filename.Sanitize(path.Ext(locator.Path))
	suffix = strings.TrimRight(strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return '_'
		}
		return r
	}, suffix), ". ")
	// Preserve the response MIME preference used by the CLI; sniff only when it
	// has no supported image type, and use the URL suffix only without a declaration.
	reported := strings.ToLower(strings.TrimSpace(strings.SplitN(response.ContentType, ";", 2)[0]))
	extension := imageExtension(reported)
	if extension == "" {
		extension = imageExtension(http.DetectContentType(header))
	}
	if extension == "" && reported == "" {
		extension = suffix
		if imageExtensionBySuffix(extension) == "" {
			extension = ""
		}
	}
	if extension == "" {
		return "", &sdk.Error{Operation: "download.Save", Reason: sdk.MalformedUpstreamResponse, Message: fmt.Sprintf("unsupported image content type %q", reported)}
	}
	if strings.EqualFold(suffix, extension) || extension == ".jpg" && strings.EqualFold(suffix, ".jpeg") {
		extension = suffix
	}
	output = filepath.Join(base, name+extension)
	if suffix == extension {
		if _, err := atomicfile.AtomicWrite(ctx, output, reader); err != nil {
			return "", err
		}
		return output, nil
	}
	// A detected suffix change must not touch a different existing download.
	if err := os.MkdirAll(base, directoryPermissions); err != nil {
		return "", err
	}
	work, err := os.MkdirTemp(base, imageDirectoryPattern)
	if err != nil {
		return "", err
	}
	defer func() { returnErr = errors.Join(returnErr, os.RemoveAll(work)) }()
	source := filepath.Join(work, imageFilename)
	if _, err := atomicfile.AtomicWrite(ctx, source, reader); err != nil {
		return "", err
	}
	if err := os.Link(source, output); err != nil {
		return "", fmt.Errorf("publish detected image extension: %w", err)
	}
	return output, nil
}
func imageExtension(mediaType string) string {
	switch mediaType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	}
	return ""
}
func imageExtensionBySuffix(extension string) string {
	switch strings.ToLower(extension) {
	case ".jpg", ".jpeg":
		return ".jpg"
	case ".png", ".gif", ".webp":
		return strings.ToLower(extension)
	}
	return ""
}
