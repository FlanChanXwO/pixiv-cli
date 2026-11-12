package ascii2d

import (
	"errors"
	"io"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

const (
	pixivHost        = "www.pixiv.net"
	pixivArtworkPath = "artworks"
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
			attribute(node, "action") != uploadPath || !strings.EqualFold(attribute(node, "method"), "post") ||
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
			case paramToken:
				if strings.EqualFold(attribute(child, "type"), "hidden") {
					formToken = strings.TrimSpace(attribute(child, "value"))
				}
			case paramFile:
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

func parseResults(body io.Reader) ([]Match, error) {
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
	// The first item-box identifies the query, including pages with no matches.
	if len(itemBoxes) == 0 {
		return nil, errors.New("query item is missing")
	}
	matches := make([]Match, 0, len(itemBoxes)-1)
	for _, item := range itemBoxes[1:] {
		match, ok, err := parseResultItem(item, len(matches)+1)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		matches = append(matches, match)
	}
	return matches, nil
}

func parseResultItem(item *html.Node, rank int) (Match, bool, error) {
	info := firstDescendantWithClass(item, "info-box")
	if info == nil {
		// Ads lack image-box and info-box; actual images still require result metadata.
		if firstDescendantWithClass(item, "image-box") == nil {
			return Match{}, false, nil
		}
		return Match{}, false, errors.New("result info is missing")
	}
	detail := firstDescendantWithClass(info, "detail-box")
	if detail == nil {
		return Match{}, false, errors.New("result detail is missing")
	}
	if external := firstDescendantWithClass(detail, "external"); external != nil {
		return parseExternalResult(external, rank)
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
				source = nodeText(node, false)
			}
		}
	})
	if len(links) == 0 || source == "" {
		return Match{}, false, errors.New("result identity is missing")
	}
	return matchFromLinks(rank, source, nodeText(links[0], false), links), true, nil
}

func parseExternalResult(external *html.Node, rank int) (Match, bool, error) {
	var links []*html.Node
	walkElements(external, func(node *html.Node) {
		if node.Data == "a" && strings.TrimSpace(attribute(node, "href")) != "" {
			links = append(links, node)
		}
	})
	if len(links) == 0 {
		return Match{}, false, errors.New("result identity is missing")
	}
	title := nodeText(external, true)
	source := nodeText(links[0], false)
	if title == "" || source == "" {
		return Match{}, false, errors.New("result identity is missing")
	}
	return matchFromLinks(rank, source, title, links), true, nil
}

func matchFromLinks(rank int, source, title string, links []*html.Node) Match {
	externalURLs := make([]string, 0, len(links))
	for _, link := range links {
		externalURLs = append(externalURLs, strings.TrimSpace(attribute(link, "href")))
	}
	match := Match{Rank: rank, IndexName: source, Title: title, ExternalURLs: externalURLs, ArtworkID: pixivArtworkID(externalURLs)}
	if len(links) > 1 {
		match.Author = nodeText(links[1], false)
	}
	return match
}

func nodeText(node *html.Node, excludeAnchors bool) string {
	var values []string
	var collect func(*html.Node)
	collect = func(current *html.Node) {
		if excludeAnchors && current.Type == html.ElementNode && current.Data == "a" {
			return
		}
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

func pixivArtworkID(links []string) int64 {
	for _, link := range links {
		parsed, err := url.Parse(link)
		if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Hostname(), pixivHost) || parsed.Port() != "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.RawPath != "" {
			continue
		}
		segments := strings.Split(strings.TrimPrefix(parsed.Path, "/"), "/")
		if len(segments) != 2 || segments[0] != pixivArtworkPath {
			continue
		}
		id, err := strconv.ParseInt(segments[1], 10, 64)
		if err == nil && id > 0 && strconv.FormatInt(id, 10) == segments[1] {
			return id
		}
	}
	return 0
}
