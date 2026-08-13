package downloader_test

import (
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	"github.com/stretchr/testify/assert"
)

func TestMimeTypeForPathReturnsSupportedImageTypes(t *testing.T) {
	assert.Equal(t, "image/jpeg", downloader.MimeTypeForPath("image.JPG"))
	assert.Equal(t, "image/png", downloader.MimeTypeForPath("image.png"))
	assert.Equal(t, "image/gif", downloader.MimeTypeForPath("image.gif"))
	assert.Equal(t, "image/webp", downloader.MimeTypeForPath("image.webp"))
	assert.Equal(t, "application/octet-stream", downloader.MimeTypeForPath("image.bin"))
}
