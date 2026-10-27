// Package plaintext 把上游 HTML 与文本转成可安全打印到终端的输出。
//
// [SafeLine] 转义换行、ANSI ESC 与其他控制字符，[HTMLPlainText] 去掉标记并保留段落
// 换行；JSON 输出由 encoding/json 自行转义，不经过本包。
package plaintext

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

// safeLines 去掉首尾空白后逐行转义整段多行文本。
func safeLines(value string) string {
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

// HTMLPlainText 把 HTML 片段转成终端可读的纯文本，并用 safeLines 转义结果。输出 JSON 的
// 调用方保留原始标记，只在展示层压平。
func HTMLPlainText(raw string) string {
	if raw == "" {
		return ""
	}
	document, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return safeLines(raw)
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
	return safeLines(out.String())
}

func appendLineBreak(out *strings.Builder) {
	if out.Len() == 0 || strings.HasSuffix(out.String(), "\n") {
		return
	}
	out.WriteByte('\n')
}
