package text

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFirstNonEmptyReturnsFirstNonEmptyString(t *testing.T) {
	assert.Equal(t, "first", FirstNonEmpty("", "first", "second"))
	assert.Empty(t, FirstNonEmpty("", ""))
	assert.Equal(t, " ", FirstNonEmpty(" ", "fallback"))
}

func TestDefaultStringReturnsFallbackForEmptyString(t *testing.T) {
	assert.Equal(t, "fallback", DefaultString("", "fallback"))
	assert.Equal(t, "value", DefaultString("value", "fallback"))
}
