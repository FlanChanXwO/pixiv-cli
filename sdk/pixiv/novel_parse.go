package pixiv

import (
	"bytes"
	"strings"
	"unicode"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"golang.org/x/net/html"
)

// ParseContent parses caller-supplied webview HTML without fetching it.
// Body images and files are validated against the client's resource policy and
// bound to its account. Missing body blocks or media locators fail explicitly.
// Each recognized media figure must contain a single independent image or file; compound
// figures fail without returning partial content.
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
				// Unknown containers can hold supported blocks that still need parsing and URL checks.
				if block.Format != NovelBlockUnknown {
					return nil
				}
			case "div":
				if strings.Contains(class, "caption") {
					content.Caption = novelText(n)
					return nil
				}
				if (strings.Contains(class, "novel") || strings.Contains(class, "text") || strings.Contains(class, "title")) && !novelWrapper(class) {
					content.Blocks = append(content.Blocks, novelUnknown(n))
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
	// A media block represents one independent resource; links and file icons belong to that resource.
	media := novelMediaElements(n)
	if (strings.Contains(class, "image") || strings.Contains(class, "file")) && len(media) > 1 {
		return NovelBlock{}, newError("Novel.ParseContent", sdk.MalformedUpstreamResponse, "compound novel media figure is unsupported", nil)
	}
	switch {
	case strings.Contains(class, "image"):
		source := novelAttribute(n, "src")
		if len(media) == 1 {
			if media[0].Data != "img" {
				return NovelBlock{}, newError("Novel.ParseContent", sdk.MalformedUpstreamResponse, "novel image contains another media type", nil)
			}
			if source == "" {
				source = novelAttribute(media[0], "src")
			}
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
		if len(media) != 1 || media[0].Data != "a" {
			return NovelBlock{}, newError("Novel.ParseContent", sdk.MalformedUpstreamResponse, "novel file has no link", nil)
		}
		link := media[0]
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
	if text := novelPlainText(n, true); text != "" {
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
			case "rt", "rp":
				return
			case "strong", "b":
				mark.Style = NovelMarkStrong
			case "em", "i":
				mark.Style = NovelMarkEmphasis
			case "del", "s", "strike":
				mark.Style = NovelMarkDelete
			case "ruby":
				ruby, nested := novelRuby(node)
				start := text.Len()
				marks = append(marks, NovelMark{Style: NovelMarkRuby, Text: ruby.Text, Start: start, End: start + len(ruby.Text), Ruby: &ruby})
				for _, child := range nested {
					child.Start += start
					child.End += start
					marks = append(marks, child)
				}
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
			marks[index].Start, marks[index].End = start, text.Len()
		}
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		walk(child)
	}
	return text.String(), marks
}

func novelRuby(n *html.Node) (NovelRuby, []NovelMark) {
	base, marks := novelInline(n)
	// Preserve ruby's trimmed base while translating nested ranges into that trimmed text.
	start := len(base) - len(strings.TrimLeftFunc(base, unicode.IsSpace))
	base = strings.TrimSpace(base)
	for i := range marks {
		marks[i].Start = min(len(base), max(0, marks[i].Start-start))
		marks[i].End = min(len(base), max(0, marks[i].End-start))
		marks[i].Text = base[marks[i].Start:marks[i].End]
	}
	var reading strings.Builder
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
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return NovelRuby{Text: base, Furigana: strings.TrimSpace(reading.String())}, marks
}

func novelText(n *html.Node) string {
	return novelPlainText(n, false)
}

func novelPlainText(n *html.Node, unclaimedOnly bool) string {
	var text strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if skipNovelNode(node) || (unclaimedOnly && node != n && novelConsumedNode(node)) {
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

func novelConsumedNode(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}
	class := novelAttribute(n, "class")
	switch n.Data {
	case "h1", "h2", "h3", "h4", "figure":
		return true
	case "p":
		return strings.Contains(class, "noveltext")
	case "div":
		return strings.Contains(class, "caption") || ((strings.Contains(class, "novel") || strings.Contains(class, "text") || strings.Contains(class, "title")) && !novelWrapper(class))
	}
	return false
}

func skipNovelNode(n *html.Node) bool {
	return n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style" || n.Data == "head")
}

func novelMediaElements(n *html.Node) []*html.Node {
	var media []*html.Node
	image := strings.Contains(novelAttribute(n, "class"), "image")
	var walk func(*html.Node, bool, bool)
	walk = func(node *html.Node, inLink, inCaption bool) {
		if skipNovelNode(node) {
			return
		}
		if node.Type == html.ElementNode {
			var independent bool
			switch node.Data {
			case "figure":
				independent = node != n
			case "img":
				independent = image || !inLink
			case "a":
				independent = (!image && !inCaption) || novelAttribute(node, "data-file-url") != ""
			}
			if independent {
				media = append(media, node)
			}
			inLink = inLink || node.Data == "a"
			inCaption = inCaption || node.Data == "figcaption"
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child, inLink, inCaption)
		}
	}
	walk(n, false, false)
	return media
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
