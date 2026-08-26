package ascii2d

import (
	"errors"
	"io"
	"strings"

	reversesearch "github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch"
	"golang.org/x/net/html"
)

func parseUploadForm(body io.Reader) (string, error) {
	document, err := html.Parse(body)
	if err != nil {
		return "", err
	}
	var token string
	var found bool
	walkElements(document, func(node *html.Node) {
		if found || node.Data != "form" || attribute(node, "id") != "file_upload" ||
			attribute(node, "action") != "/search/file" || !strings.EqualFold(attribute(node, "method"), "post") ||
			!strings.EqualFold(attribute(node, "enctype"), "multipart/form-data") {
			return
		}
		var formToken string
		hasFile := false
		walkElements(node, func(child *html.Node) {
			if child.Data != "input" {
				return
			}
			switch attribute(child, "name") {
			case "authenticity_token":
				if strings.EqualFold(attribute(child, "type"), "hidden") {
					formToken = strings.TrimSpace(attribute(child, "value"))
				}
			case "file":
				hasFile = hasFile || strings.EqualFold(attribute(child, "type"), "file")
			}
		})
		found = formToken != "" && hasFile
		if found {
			token = formToken
		}
	})
	if !found {
		return "", errors.New("upload form is missing")
	}
	return token, nil
}

func parseResults(body io.Reader) ([]reversesearch.Match, error) {
	document, err := html.Parse(body)
	if err != nil {
		return nil, err
	}
	var itemBoxes []*html.Node
	walkElements(document, func(node *html.Node) {
		if hasClass(node, "item-box") {
			itemBoxes = append(itemBoxes, node)
		}
	})
	// ascii2d 的首个 item-box 是查询图片；即使没有命中也必须存在。
	if len(itemBoxes) == 0 {
		return nil, errors.New("query item is missing")
	}
	matches := make([]reversesearch.Match, 0, len(itemBoxes)-1)
	for _, item := range itemBoxes[1:] {
		info := firstDescendantWithClass(item, "info-box")
		if info == nil {
			return nil, errors.New("result info is missing")
		}
		detail := firstDescendantWithClass(info, "detail-box")
		if detail == nil {
			return nil, errors.New("result detail is missing")
		}
		var links []*html.Node
		var source string
		walkElements(detail, func(node *html.Node) {
			switch node.Data {
			case "a":
				if strings.TrimSpace(attribute(node, "href")) != "" {
					links = append(links, node)
				}
			case "small":
				if source == "" {
					source = nodeText(node)
				}
			}
		})
		if len(links) == 0 || source == "" {
			return nil, errors.New("result identity is missing")
		}
		externalURLs := make([]string, 0, len(links))
		for _, link := range links {
			externalURLs = append(externalURLs, strings.TrimSpace(attribute(link, "href")))
		}
		match := reversesearch.Match{
			Rank: len(matches) + 1, IndexName: source, Title: nodeText(links[0]),
			ExternalURLs: externalURLs,
		}
		if len(links) > 1 {
			match.Author = nodeText(links[1])
		}
		matches = append(matches, match)
	}
	return matches, nil
}

func walkElements(node *html.Node, visit func(*html.Node)) {
	if node.Type == html.ElementNode {
		visit(node)
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		walkElements(child, visit)
	}
}

func firstDescendantWithClass(node *html.Node, class string) *html.Node {
	var found *html.Node
	walkElements(node, func(candidate *html.Node) {
		if found == nil && candidate != node && hasClass(candidate, class) {
			found = candidate
		}
	})
	return found
}

func hasClass(node *html.Node, class string) bool {
	for _, candidate := range strings.Fields(attribute(node, "class")) {
		if candidate == class {
			return true
		}
	}
	return false
}

func attribute(node *html.Node, key string) string {
	for _, attribute := range node.Attr {
		if strings.EqualFold(attribute.Key, key) {
			return attribute.Val
		}
	}
	return ""
}

func nodeText(node *html.Node) string {
	var values []string
	var collect func(*html.Node)
	collect = func(current *html.Node) {
		if current.Type == html.TextNode {
			values = append(values, current.Data)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			collect(child)
		}
	}
	collect(node)
	return strings.Join(strings.Fields(strings.Join(values, " ")), " ")
}
