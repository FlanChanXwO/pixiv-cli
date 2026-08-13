package text

import (
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// SafeLine escapes newlines, ANSI ESC and other control bytes while keeping
// printable Unicode. Line-oriented terminal output uses it so upstream text
// cannot break the one-record-per-line protocol. JSON output stays untouched
// because encoding/json performs its own escaping.
func SafeLine(value string) string {
	quoted := strconv.QuoteToGraphic(value)
	return quoted[1 : len(quoted)-1]
}

// SafeLines trims and escapes every line of a multi-line block.
func SafeLines(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	lines := strings.Split(value, "\n")
	for index, line := range lines {
		lines[index] = SafeLine(strings.TrimSpace(line))
	}
	return strings.Join(lines, "\n")
}

// HTMLPlainText renders an HTML fragment as terminal-readable plain text and
// escapes the result with SafeLines. Callers that emit JSON keep the original
// markup; only the presentation layer flattens it.
func HTMLPlainText(raw string) string {
	if raw == "" {
		return ""
	}
	document, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return SafeLines(raw)
	}
	var out strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		switch node.Type {
		case html.TextNode:
			out.WriteString(node.Data)
			return
		case html.ElementNode:
			switch node.Data {
			case "script", "style":
				return
			case "br":
				appendLineBreak(&out)
				return
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if node.Type == html.ElementNode {
			switch node.Data {
			case "p", "div", "li", "blockquote", "pre", "h1", "h2", "h3", "h4", "h5", "h6":
				appendLineBreak(&out)
			}
		}
	}
	walk(document)
	return SafeLines(out.String())
}

func appendLineBreak(out *strings.Builder) {
	if out.Len() == 0 || strings.HasSuffix(out.String(), "\n") {
		return
	}
	out.WriteByte('\n')
}
