package ascii2d

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"golang.org/x/net/html"
)

func detectChallenge(status int, header http.Header, data []byte) (bool, error) {
	// The cf-mitigated header is authoritative regardless of HTTP status.
	if challengeHeaderDetected(header) {
		return true, nil
	}
	if status != http.StatusForbidden || !isHTMLContentType(header) {
		return false, nil
	}
	return scanHTMLChallenge(bytes.NewReader(data))
}

func challengeHeaderDetected(header http.Header) bool {
	for _, value := range header.Values(headerMitigated) {
		for _, item := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(item), "challenge") {
				return true
			}
		}
	}
	return false
}

func isHTMLContentType(header http.Header) bool {
	mediaType, _, err := mime.ParseMediaType(header.Get(headerContentType))
	if err != nil {
		return false
	}
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	return mediaType == "text/html" || mediaType == "application/xhtml+xml" || strings.HasSuffix(mediaType, "+html")
}

func scanHTMLChallenge(body io.Reader) (bool, error) {
	tokenizer := html.NewTokenizer(body)
	activeHeading := ""
	var headingText strings.Builder
	for {
		switch tokenType := tokenizer.Next(); tokenType {
		case html.ErrorToken:
			if errors.Is(tokenizer.Err(), io.EOF) {
				return false, nil
			}
			return false, tokenizer.Err()
		case html.StartTagToken, html.SelfClosingTagToken:
			token := tokenizer.Token()
			if tokenHasChallengeMarker(token) {
				return true, nil
			}
			if isChallengeHeading(token.Data) {
				activeHeading = token.Data
				headingText.Reset()
			}
		case html.EndTagToken:
			token := tokenizer.Token()
			if token.Data == activeHeading {
				activeHeading = ""
			}
		case html.TextToken, html.CommentToken:
			token := tokenizer.Token()
			text := normalizeChallengeText(token.Data)
			if hasChallengeText(text) {
				return true, nil
			}
			if activeHeading != "" {
				headingText.WriteString(" ")
				headingText.WriteString(text)
				if hasChallengeText(normalizeChallengeText(headingText.String())) {
					return true, nil
				}
			}
		}
	}
}

func tokenHasChallengeMarker(token html.Token) bool {
	for _, attribute := range token.Attr {
		value := strings.ToLower(attribute.Key + "=" + attribute.Val)
		if strings.Contains(value, "cf-chl") || strings.Contains(value, "cf_chl") || strings.Contains(value, "challenge-platform") {
			return true
		}
	}
	return false
}

func isChallengeHeading(name string) bool {
	switch strings.ToLower(name) {
	case "title", "h1", "h2":
		return true
	default:
		return false
	}
}

func normalizeChallengeText(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(value)), " ")
}

func hasChallengeText(value string) bool {
	// Generic WAF errors and Ray IDs do not establish a solvable challenge.
	for _, marker := range []string{
		"just a moment",
		"verify you are human",
		"checking your browser before accessing",
		"challenge-platform",
		"cf-chl-",
		"cf_chl",
	} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}
