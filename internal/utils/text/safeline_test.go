package text_test

import (
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/utils/text"
	"github.com/stretchr/testify/assert"
)

func TestSafeLineEscapesControlBytesAndKeepsPrintableUnicode(t *testing.T) {
	assert.Equal(t, `first\nsecond`, text.SafeLine("first\nsecond"))
	assert.Equal(t, `\x1b[31mred`, text.SafeLine("\x1b[31mred"))
	assert.Equal(t, "初音ミク", text.SafeLine("初音ミク"))
	assert.Empty(t, text.SafeLine(""))
}

func TestSafeLinesTrimsAndEscapesEveryLine(t *testing.T) {
	assert.Equal(t, "one\ntwo", text.SafeLines("  one  \n  two  "))
	assert.Empty(t, text.SafeLines("   "))
	assert.Equal(t, `a\tb`, text.SafeLines("a\tb"))
}

func TestHTMLPlainTextFlattensMarkupAndDropsScriptContent(t *testing.T) {
	assert.Equal(t, "line one\nline two", text.HTMLPlainText("line one<br>line two"))
	assert.Equal(t, "paragraph", text.HTMLPlainText("<p>paragraph</p>"))
	assert.Equal(t, "visible", text.HTMLPlainText("<div>visible</div><script>hidden()</script>"))
	assert.Empty(t, text.HTMLPlainText(""))
}

func TestHTMLPlainTextEscapesControlBytesFromUpstreamMarkup(t *testing.T) {
	assert.Equal(t, `\x1b[31mred`, text.HTMLPlainText("<p>\x1b[31mred</p>"))
}
