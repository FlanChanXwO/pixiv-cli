package download

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/media/filename"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/atomicfile"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

const imageSniffBytes = 512 // http.DetectContentType examines at most this many bytes.

func saveImage(ctx context.Context, image pixiv.Image, base, name string) (output string, returnErr error) {
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
	locator, err := url.Parse(image.URL)
	if err != nil {
		return "", invalidArgument("invalid image URL")
	}
	// Preserve the response MIME preference used by the CLI; sniff only when it
	// has no supported image type, and use the URL suffix only without a declaration.
	reported := strings.ToLower(strings.TrimSpace(strings.SplitN(response.ContentType, ";", 2)[0]))
	extension := imageExtension(reported)
	if extension == "" {
		extension = imageExtension(http.DetectContentType(header))
	}
	if extension == "" && reported == "" {
		extension = filename.Sanitize(path.Ext(locator.Path))
		extension = strings.TrimRight(strings.Map(func(r rune) rune {
			if r < 0x20 || r == 0x7f {
				return '_'
			}
			return r
		}, extension), ". ")
		if imageExtensionBySuffix(extension) == "" {
			extension = ""
		}
	}
	if extension == "" {
		return "", &sdk.Error{Operation: "download.Save", Reason: sdk.MalformedUpstreamResponse, Message: fmt.Sprintf("unsupported image content type %q", reported)}
	}
	if extension == ".jpg" && strings.EqualFold(path.Ext(locator.Path), ".jpeg") {
		extension = path.Ext(locator.Path)
	}
	output = filepath.Join(base, name+extension)
	if _, err := atomicfile.AtomicWrite(ctx, output, reader); err != nil {
		return "", err
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
