package plaintext_test

import (
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/plaintext"
	"github.com/stretchr/testify/assert"
)

func TestFirstNonEmptyReturnsFirstNonEmptyString(t *testing.T) {
	assert.Equal(t, "first", plaintext.FirstNonEmpty("", "first", "second"))
	assert.Empty(t, plaintext.FirstNonEmpty("", ""))
	assert.Equal(t, " ", plaintext.FirstNonEmpty(" ", "fallback"))
}

func TestDefaultStringReturnsFallbackForEmptyString(t *testing.T) {
	assert.Equal(t, "fallback", plaintext.DefaultString("", "fallback"))
	assert.Equal(t, "value", plaintext.DefaultString("value", "fallback"))
}
