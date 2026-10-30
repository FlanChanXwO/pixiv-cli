package display

import (
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// SafeLine 转义换行、ANSI ESC 与其他控制字节，保留可打印 Unicode。逐行终端输出依赖
// 它防止上游文本破坏一条记录一行的协议；JSON 输出由 encoding/json 自行转义，不经过
// 这里。
func SafeLine(value string) string {
	quoted := strconv.QuoteToGraphic(value)
	return quoted[1 : len(quoted)-1]
}

// htmlPlainText 把 HTML 片段转成终端可读的纯文本，并转义为单行。输出 JSON 的
// 调用方保留原始标记，只在展示层压平。
func htmlPlainText(raw string) string {
	if raw == "" {
		return ""
	}
	document, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return SafeLine(strings.TrimSpace(raw))
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
	return SafeLine(strings.TrimSpace(out.String()))
}

func appendLineBreak(out *strings.Builder) {
	if out.Len() == 0 || strings.HasSuffix(out.String(), "\n") {
		return
	}
	out.WriteByte('\n')
}
