package uri_test

import (
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/utils/uri"
	"github.com/stretchr/testify/assert"
)

func TestPathFromURLReturnsParsedPathWhenURLIsValid(t *testing.T) {
	assert.Equal(t, "/img-original/file.jpg", uri.PathFromURL("https://i.pximg.net/img-original/file.jpg?x=1"))
	assert.Equal(t, "/img-original/a b.jpg", uri.PathFromURL("https://i.pximg.net/img-original/a%20b.jpg"))
	assert.Equal(t, "/img-original/file.jpg", uri.PathFromURL("https://i.pximg.net/img-original/file%2Ejpg"))
	assert.Equal(t, "not a url", uri.PathFromURL("not a url"))
	assert.Equal(t, "https://i.pximg.net?x=1", uri.PathFromURL("https://i.pximg.net?x=1"))
}

func TestFileURIBuildsAbsoluteFileURI(t *testing.T) {
	got := uri.FileURI(filepath.Join("downloads", "image 1.jpg"))

	assert.Contains(t, got, "file://")
	assert.Contains(t, got, "downloads/image%201.jpg")
}
