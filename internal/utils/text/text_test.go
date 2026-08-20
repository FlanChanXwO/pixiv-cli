package text_test

import (
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/utils/text"
	"github.com/stretchr/testify/assert"
)

func TestFirstNonEmptyReturnsFirstNonEmptyString(t *testing.T) {
	assert.Equal(t, "first", text.FirstNonEmpty("", "first", "second"))
	assert.Empty(t, text.FirstNonEmpty("", ""))
	assert.Equal(t, " ", text.FirstNonEmpty(" ", "fallback"))
}

func TestDefaultStringReturnsFallbackForEmptyString(t *testing.T) {
	assert.Equal(t, "fallback", text.DefaultString("", "fallback"))
	assert.Equal(t, "value", text.DefaultString("value", "fallback"))
}
