package downloader_test

import (
	"path/filepath"
	"testing"

	downloader "github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	"github.com/stretchr/testify/assert"
)

func TestPathFromURLReturnsParsedPathWhenURLIsValid(t *testing.T) {
	assert.Equal(t, "/img-original/file.jpg", downloader.PathFromURL("https://i.pximg.net/img-original/file.jpg?x=1"))
	assert.Equal(t, "/img-original/a b.jpg", downloader.PathFromURL("https://i.pximg.net/img-original/a%20b.jpg"))
	assert.Equal(t, "/img-original/file.jpg", downloader.PathFromURL("https://i.pximg.net/img-original/file%2Ejpg"))
	assert.Equal(t, "not a url", downloader.PathFromURL("not a url"))
	assert.Equal(t, "https://i.pximg.net?x=1", downloader.PathFromURL("https://i.pximg.net?x=1"))
}

func TestFileURIBuildsAbsoluteFileURI(t *testing.T) {
	got := downloader.FileURI(filepath.Join("downloads", "image 1.jpg"))

	assert.Contains(t, got, "file://")
	assert.Contains(t, got, "downloads/image%201.jpg")
}
