package pixiv

import (
	"bytes"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"golang.org/x/net/html"
)

// ParseContent parses caller-supplied webview HTML without fetching it.
// Body images and files are validated against the client's resource policy and
// bound to its account. Missing body blocks or media locators fail explicitly.
func (r *NovelRef) ParseContent(raw []byte) (NovelContent, error) {
	// 1. Establish the resource owner and parse the supplied document.
	if err := r.bound("Novel.ParseContent"); err != nil {
		return NovelContent{}, err
	}
	doc, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		return NovelContent{}, newError("Novel.ParseContent", sdk.MalformedUpstreamResponse, "unparseable webview HTML", err)
	}
	// 2. Consume each block once, preserving its nested inline formatting.
	content := NovelContent{NovelID: r.ID, Blocks: []NovelBlock{}}
	var walk func(*html.Node) error
	walk = func(n *html.Node) error {
		if skipNovelNode(n) {
			return nil
		}
		class := novelAttribute(n, "class")
		if n.Type == html.ElementNode {
			switch n.Data {
			case "h1", "h2", "h3", "h4":
				if n.Data == "h1" && content.Title == "" && strings.Contains(class, "title") {
					content.Title = novelText(n)
					return nil
				}
				text, marks := novelInline(n)
				content.Blocks = append(content.Blocks, NovelBlock{Format: NovelBlockHeader, Text: text, Marks: marks})
				return nil
			case "p":
				if strings.Contains(class, "noveltext") {
					text, marks := novelInline(n)
					content.Blocks = append(content.Blocks, NovelBlock{Format: NovelBlockParagraph, Text: text, Marks: marks})
					return nil
				}
			case "figure":
				block, err := r.client.novelMediaBlock(n)
				if err != nil {
					return err
				}
				content.Blocks = append(content.Blocks, block)
				return nil
			case "div":
				if strings.Contains(class, "caption") {
					content.Caption = novelText(n)
					return nil
				}
				if (strings.Contains(class, "novel") || strings.Contains(class, "text") || strings.Contains(class, "title")) && !novelWrapper(class) {
					content.Blocks = append(content.Blocks, novelUnknown(n))
					return nil
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(doc); err != nil {
		return NovelContent{}, err
	}
	// 3. Reject documents that contain no recognizable body content.
	if len(content.Blocks) == 0 {
		return NovelContent{}, newError("Novel.ParseContent", sdk.MalformedUpstreamResponse, "webview contains no body blocks", nil)
	}
	return content, nil
}

func (c *Client) novelMediaBlock(n *html.Node) (NovelBlock, error) {
	class := novelAttribute(n, "class")
	switch {
	case strings.Contains(class, "image"):
		source := novelAttribute(n, "src")
		if image := novelElement(n, "img"); source == "" && image != nil {
			source = novelAttribute(image, "src")
		}
		if source == "" {
			return NovelBlock{}, newError("Novel.ParseContent", sdk.MalformedUpstreamResponse, "novel image has no source", nil)
		}
		image, err := c.image(source, 0, 0)
		if err != nil {
			return NovelBlock{}, err
		}
		return NovelBlock{Format: NovelBlockImage, Marks: []NovelMark{}, Image: &NovelImageBlock{Image: image, Caption: novelText(n)}}, nil
	case strings.Contains(class, "file"):
		link := novelElement(n, "a")
		if link == nil {
			return NovelBlock{}, newError("Novel.ParseContent", sdk.MalformedUpstreamResponse, "novel file has no link", nil)
		}
		source := novelAttribute(link, "href")
		if source == "" {
			source = novelAttribute(link, "data-file-url")
		}
		if source == "" {
			return NovelBlock{}, newError("Novel.ParseContent", sdk.MalformedUpstreamResponse, "novel file has no source", nil)
		}
		if err := c.app(Credentials{}).ValidateResourceURL(source); err != nil {
			return NovelBlock{}, newError("Novel.ParseContent", sdk.ResourceForbidden, "file URL is not allowed", nil)
		}
		file := File{URL: source, Filename: novelText(link), client: c}
		return NovelBlock{Format: NovelBlockFile, Marks: []NovelMark{}, File: &NovelFileBlock{File: file, Caption: novelText(n)}}, nil
	default:
		return novelUnknown(n), nil
	}
}

func novelUnknown(n *html.Node) NovelBlock {
	payload := map[string]string{}
	if text := novelText(n); text != "" {
		payload["text"] = text
	}
	return NovelBlock{Format: NovelBlockUnknown, Marks: []NovelMark{}, Unknown: &NovelUnknownBlock{Source: n.Data + ":" + novelAttribute(n, "class"), Payload: payload}}
}

func novelInline(n *html.Node) (string, []NovelMark) {
	var text strings.Builder
	marks := []NovelMark{}
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if skipNovelNode(node) {
			return
		}
		if node.Type == html.TextNode {
			text.WriteString(node.Data)
			return
		}
		if node.Type == html.ElementNode && node.Data == "br" {
			text.WriteByte('\n')
			return
		}
		mark := NovelMark{}
		if node.Type == html.ElementNode {
			switch node.Data {
			case "strong", "b":
				mark.Style = NovelMarkStrong
			case "em", "i":
				mark.Style = NovelMarkEmphasis
			case "del", "s", "strike":
				mark.Style = NovelMarkDelete
			case "ruby":
				ruby := novelRuby(node)
				marks = append(marks, NovelMark{Style: NovelMarkRuby, Text: ruby.Text, Ruby: &ruby})
				text.WriteString(ruby.Text)
				return
			case "a":
				mark.Style = NovelMarkLink
				mark.Href = novelAttribute(node, "href")
			case "span":
				mark.Style = NovelMarkUnknown
				mark.Class = novelAttribute(node, "class")
				if strings.Contains(mark.Class, "novelcustom") {
					mark.Style = NovelMarkCustom
				}
			}
		}
		// Reserve the parent's mark before walking children to preserve nested document order.
		index, start := len(marks), text.Len()
		if mark.Style != "" {
			marks = append(marks, mark)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if mark.Style != "" {
			marks[index].Text = text.String()[start:]
		}
	}
	walk(n)
	return text.String(), marks
}

func novelRuby(n *html.Node) NovelRuby {
	var base, reading strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if skipNovelNode(node) {
			return
		}
		if node.Type == html.ElementNode {
			if node.Data == "rt" {
				reading.WriteString(novelText(node))
				return
			}
			if node.Data == "rp" {
				return
			}
		}
		if node.Type == html.TextNode {
			base.WriteString(node.Data)
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return NovelRuby{Text: strings.TrimSpace(base.String()), Furigana: strings.TrimSpace(reading.String())}
}

func novelText(n *html.Node) string {
	var text strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if skipNovelNode(node) {
			return
		}
		if node.Type == html.TextNode {
			text.WriteString(node.Data)
			return
		}
		if node.Type == html.ElementNode && node.Data == "br" {
			text.WriteByte('\n')
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return strings.TrimSpace(text.String())
}

func skipNovelNode(n *html.Node) bool {
	return n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style" || n.Data == "head")
}

func novelElement(n *html.Node, name string) *html.Node {
	if skipNovelNode(n) {
		return nil
	}
	if n.Type == html.ElementNode && n.Data == name {
		return n
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if found := novelElement(child, name); found != nil {
			return found
		}
	}
	return nil
}

func novelAttribute(n *html.Node, key string) string {
	for _, attribute := range n.Attr {
		if strings.EqualFold(attribute.Key, key) {
			return attribute.Val
		}
	}
	return ""
}

func novelWrapper(class string) bool {
	for _, wrapper := range []string{"novel-view", "novel-body", "novel-caption", "novel-title", "novel-header", "novel-footer", "novel-content", "novel-info", "novel-wrapper"} {
		if strings.Contains(class, wrapper) {
			return true
		}
	}
	return false
}
