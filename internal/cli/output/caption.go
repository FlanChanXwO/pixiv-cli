package output

import (
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// CaptionPlainText 将 Pixiv 的 HTML caption 转为终端可读文本。JSON 仍保留原始
// caption；这里只处理展示层，并转义控制字符以免上游文本破坏终端逐行协议。
func CaptionPlainText(raw string) string {
	if raw == "" {
		return ""
	}
	document, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return safeCaptionLines(raw)
	}
	var text strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		switch node.Type {
		case html.TextNode:
			text.WriteString(node.Data)
			return
		case html.ElementNode:
			switch node.Data {
			case "script", "style":
				return
			case "br":
				appendCaptionLineBreak(&text)
				return
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if node.Type == html.ElementNode {
			switch node.Data {
			case "p", "div", "li", "blockquote", "pre", "h1", "h2", "h3", "h4", "h5", "h6":
				appendCaptionLineBreak(&text)
			}
		}
	}
	walk(document)
	return safeCaptionLines(text.String())
}

func appendCaptionLineBreak(out *strings.Builder) {
	if out.Len() == 0 || strings.HasSuffix(out.String(), "\n") {
		return
	}
	out.WriteByte('\n')
}

func safeCaptionLines(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	lines := strings.Split(value, "\n")
	for index, line := range lines {
		lines[index] = SafeTextLine(strings.TrimSpace(line))
	}
	return strings.Join(lines, "\n")
}

// SafeTextLine 转义换行、ANSI ESC 和其他控制字节，保留可见 Unicode，
// 防止上游文本破坏终端的逐行文本协议。JSON 输出仍交由 encoding/json 原样编码。
func SafeTextLine(value string) string {
	quoted := strconv.QuoteToGraphic(value)
	return quoted[1 : len(quoted)-1]
}
