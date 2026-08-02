package pixiv

import (
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"golang.org/x/net/html"
)

// parseNovelContent converts the official novel webview HTML into the
// structured block model. It never exposes raw HTML: unparseable content fails
// with CodeMalformedUpstreamResponse rather than returning a partial body.
func (c *Client) parseNovelContent(novelID int64, raw []byte) (NovelContent, error) {
	doc, err := html.Parse(strings.NewReader(string(raw)))
	if err != nil {
		return NovelContent{}, newError("NovelContent", sdk.CodeMalformedUpstreamResponse, "novel webview HTML is unparseable")
	}
	title, caption, blocks, err := c.collectNovelBody(doc, novelID)
	if err != nil {
		return NovelContent{}, err
	}
	if len(blocks) == 0 {
		return NovelContent{}, newError("NovelContent", sdk.CodeMalformedUpstreamResponse, "novel webview contains no body blocks")
	}
	return NovelContent{NovelID: novelID, Title: title, Caption: caption, Blocks: blocks}, nil
}

func (c *Client) collectNovelBody(doc *html.Node, novelID int64) (title, caption string, blocks []NovelBlock, err error) {
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if err != nil {
			return
		}
		if n.Type == html.ElementNode {
			switch n.Data {
			case "h1", "h2", "h3", "h4":
				if n.Data == "h1" && title == "" && strings.Contains(blockClass(n), "title") {
					title = textContent(n)
				} else {
					var block NovelBlock
					block, err = c.textBlock(n, NovelBlockHeader)
					blocks = append(blocks, block)
				}
			case "p":
				if isNovelText(n) {
					var block NovelBlock
					block, err = c.textBlock(n, NovelBlockParagraph)
					blocks = append(blocks, block)
				}
			case "figure":
				kind := blockClass(n)
				switch {
				case strings.Contains(kind, "image"):
					var block NovelBlock
					block, err = c.imageBlock(n, novelID)
					blocks = append(blocks, block)
				case strings.Contains(kind, "file"):
					var block NovelBlock
					block, err = c.fileBlock(n, novelID)
					blocks = append(blocks, block)
				default:
					var block NovelBlock
					block, err = c.unknownBlock(n)
					blocks = append(blocks, block)
				}
			case "div":
				if isNovelCaption(n) {
					caption = textContent(n)
				} else if isNovelBlockNode(n) && !isStructuralNovelWrapper(blockClass(n)) {
					var block NovelBlock
					block, err = c.unknownBlock(n)
					blocks = append(blocks, block)
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return title, caption, blocks, err
}

func (c *Client) textBlock(n *html.Node, kind NovelBlockKind) (NovelBlock, error) {
	text, marks, err := inlineContent(n)
	if err != nil {
		return NovelBlock{}, err
	}
	return NovelBlock{Kind: kind, Text: text, Marks: marks}, nil
}

func (c *Client) imageBlock(n *html.Node, novelID int64) (NovelBlock, error) {
	url := firstAttribute(n, "src")
	if url == "" {
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			url = firstAttribute(child, "src")
			if url != "" {
				break
			}
		}
	}
	if url == "" {
		return NovelBlock{}, newError("NovelContent", sdk.CodeMalformedUpstreamResponse, "novel image block has no source")
	}
	res, err := c.newResource("novel_image", novelID, -1, url)
	if err != nil {
		return NovelBlock{}, err
	}
	return NovelBlock{Kind: NovelBlockImage, Image: &NovelImageBlock{Resource: res, Caption: textContent(n)}}, nil
}

func (c *Client) fileBlock(n *html.Node, novelID int64) (NovelBlock, error) {
	href, filename := "", ""
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.Data == "a" {
			href = firstAttribute(child, "href")
			filename = textContent(child)
			if href == "" {
				href = firstAttribute(child, "data-file-url")
			}
			break
		}
	}
	if href == "" {
		return NovelBlock{}, newError("NovelContent", sdk.CodeMalformedUpstreamResponse, "novel file block has no link")
	}
	res, err := c.newResource("novel_file", novelID, -1, href)
	if err != nil {
		return NovelBlock{}, err
	}
	return NovelBlock{Kind: NovelBlockFile, File: &NovelFileBlock{Resource: res, Filename: filename, Caption: textContent(n)}}, nil
}

func (c *Client) unknownBlock(n *html.Node) (NovelBlock, error) {
	block := NovelBlock{Kind: NovelBlockUnknown}
	unknown := &NovelUnknownBlock{RawType: n.Data + ":" + blockClass(n)}
	text := textContent(n)
	if text != "" {
		unknown.Payload = map[string]string{"text": text}
	}
	block.Unknown = unknown
	return block, nil
}

func inlineContent(n *html.Node) (string, []NovelMark, error) {
	var builder strings.Builder
	var marks []NovelMark
	var walk func(*html.Node) error
	walk = func(node *html.Node) error {
		switch node.Type {
		case html.TextNode:
			builder.WriteString(node.Data)
		case html.ElementNode:
			switch node.Data {
			case "strong", "b":
				text, err := inlineText(node)
				if err != nil {
					return err
				}
				marks = append(marks, NovelMark{Kind: NovelMarkStrong, Text: text})
				builder.WriteString(text)
			case "em", "i":
				text, err := inlineText(node)
				if err != nil {
					return err
				}
				marks = append(marks, NovelMark{Kind: NovelMarkEmphasis, Text: text})
				builder.WriteString(text)
			case "del", "s", "strike":
				text, err := inlineText(node)
				if err != nil {
					return err
				}
				marks = append(marks, NovelMark{Kind: NovelMarkDelete, Text: text})
				builder.WriteString(text)
			case "ruby":
				base, furigana := rubyText(node)
				marks = append(marks, NovelMark{Kind: NovelMarkRuby, Text: base, Ruby: &NovelRuby{Text: base, Furigana: furigana}})
				builder.WriteString(base)
			case "a":
				text, err := inlineText(node)
				if err != nil {
					return err
				}
				href := firstAttribute(node, "href")
				marks = append(marks, NovelMark{Kind: NovelMarkLink, Text: text, Href: href})
				builder.WriteString(text)
			case "span":
				class := firstAttribute(node, "class")
				text, err := inlineText(node)
				if err != nil {
					return err
				}
				kind := NovelMarkCustom
				if !strings.Contains(class, "novelcustom") {
					kind = NovelMarkUnknown
				}
				marks = append(marks, NovelMark{Kind: kind, Text: text, Class: class})
				builder.WriteString(text)
			case "br":
				builder.WriteString("\n")
			default:
				for child := node.FirstChild; child != nil; child = child.NextSibling {
					if err := walk(child); err != nil {
						return err
					}
				}
			}
		default:
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				if err := walk(child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(n); err != nil {
		return "", nil, err
	}
	return builder.String(), marks, nil
}

func inlineText(n *html.Node) (string, error) {
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			builder.WriteString(node.Data)
			return
		}
		if node.Type == html.ElementNode && node.Data == "br" {
			builder.WriteString("\n")
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return builder.String(), nil
}

func rubyText(n *html.Node) (base, furigana string) {
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "rt" {
			furigana = textContent(node)
			return
		}
		if node.Type == html.ElementNode && (node.Data == "rp" || node.Data == "rt") {
			return
		}
		if node.Type == html.TextNode {
			base += node.Data
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return strings.TrimSpace(base), strings.TrimSpace(furigana)
}

func textContent(n *html.Node) string {
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			builder.WriteString(node.Data)
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return strings.TrimSpace(builder.String())
}

func firstAttribute(n *html.Node, key string) string {
	if n == nil {
		return ""
	}
	for _, attr := range n.Attr {
		if strings.EqualFold(attr.Key, key) {
			return attr.Val
		}
	}
	return ""
}

func blockClass(n *html.Node) string {
	return firstAttribute(n, "class")
}

func isNovelText(n *html.Node) bool {
	return strings.Contains(blockClass(n), "noveltext")
}

func isNovelCaption(n *html.Node) bool {
	return strings.Contains(blockClass(n), "caption")
}

func isNovelBlockNode(n *html.Node) bool {
	class := blockClass(n)
	return strings.Contains(class, "novel") || strings.Contains(class, "text") || strings.Contains(class, "title")
}

// isStructuralNovelWrapper reports whether a class names a layout wrapper rather
// than a content block, so structural divs are never captured as unknown blocks.
func isStructuralNovelWrapper(class string) bool {
	for _, wrapper := range []string{
		"novel-view", "novel-body", "novel-caption", "novel-title",
		"novel-header", "novel-footer", "novel-content", "novel-info", "novel-wrapper",
	} {
		if strings.Contains(class, wrapper) {
			return true
		}
	}
	return false
}
