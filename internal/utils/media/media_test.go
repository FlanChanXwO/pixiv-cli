package media

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMimeTypeForPathReturnsSupportedImageTypes(t *testing.T) {
	assert.Equal(t, "image/jpeg", MimeTypeForPath("image.JPG"))
	assert.Equal(t, "image/png", MimeTypeForPath("image.png"))
	assert.Equal(t, "image/gif", MimeTypeForPath("image.gif"))
	assert.Equal(t, "image/webp", MimeTypeForPath("image.webp"))
	assert.Equal(t, "application/octet-stream", MimeTypeForPath("image.bin"))
}
