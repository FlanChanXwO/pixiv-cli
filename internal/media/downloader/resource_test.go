package downloader_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	"github.com/stretchr/testify/assert"
)

func TestMimeTypeForFileUsesImageSignature(t *testing.T) {
	path := filepath.Join(t.TempDir(), "thumbnail.png")
	assert.NoError(t, os.WriteFile(path, []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00"), 0o600))
	assert.Equal(t, "image/jpeg", downloader.MimeTypeForFile(path))
}

func TestFileURIBuildsAbsoluteFileURI(t *testing.T) {
	got := downloader.FileURI(filepath.Join("downloads", "image 1.jpg"))

	assert.Contains(t, got, "file://")
	assert.Contains(t, got, "downloads/image%201.jpg")
}
