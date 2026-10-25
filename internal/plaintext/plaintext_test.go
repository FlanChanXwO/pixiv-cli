package plaintext_test

import (
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/plaintext"
	"github.com/stretchr/testify/assert"
)

func TestSafeLineEscapesControlBytesAndKeepsPrintableUnicode(t *testing.T) {
	assert.Equal(t, `first\nsecond`, plaintext.SafeLine("first\nsecond"))
	assert.Equal(t, `\x1b[31mred`, plaintext.SafeLine("\x1b[31mred"))
	assert.Equal(t, "初音ミク", plaintext.SafeLine("初音ミク"))
	assert.Empty(t, plaintext.SafeLine(""))
}

func TestHTMLPlainTextFlattensMarkupAndDropsScriptContent(t *testing.T) {
	assert.Equal(t, "line one\nline two", plaintext.HTMLPlainText("line one<br>line two"))
	assert.Equal(t, "paragraph", plaintext.HTMLPlainText("<p>paragraph</p>"))
	assert.Equal(t, "visible", plaintext.HTMLPlainText("<div>visible</div><script>hidden()</script>"))
	assert.Equal(t, "one\ntwo", plaintext.HTMLPlainText("  one  \n  two  "))
	assert.Empty(t, plaintext.HTMLPlainText(""))
	assert.Equal(t, `\x1b[31mred`, plaintext.HTMLPlainText("<p>\x1b[31mred</p>"))
}
