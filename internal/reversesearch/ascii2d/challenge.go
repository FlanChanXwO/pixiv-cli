package ascii2d

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	reversesearch "github.com/FlanChanXwO/pixiv-cli/internal/reversesearch"
	"golang.org/x/net/html"
)

func classifyResponseError(ctx context.Context, response *http.Response, fallbackMessage string) error {
	if challengeErr := detectResponseChallenge(ctx, response); challengeErr != nil {
		return challengeErr
	}
	return reversesearch.NewError(reversesearch.CodeUpstreamHTTPStatus, fallbackMessage, nil)
}

func detectResponseChallenge(ctx context.Context, response *http.Response) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	challenged, readErr := detectChallenge(response)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if readErr != nil {
		return reversesearch.NewError(reversesearch.CodeProviderFailed, "could not read ascii2d error response", readErr)
	}
	if challenged {
		return reversesearch.NewError(reversesearch.CodeUpstreamHTTPStatus, "ascii2d challenge detected", errChallengeDetected)
	}
	return nil
}

func detectChallenge(response *http.Response) (bool, error) {
	// cf-mitigated 是 Cloudflare 明确声明 challenge 的权威信号，优先于正文。
	if challengeHeaderDetected(response.Header) {
		return true, nil
	}
	if response.StatusCode != http.StatusForbidden || !isHTMLContentType(response.Header) {
		return false, nil
	}
	return scanHTMLChallenge(response.Body)
}

func challengeHeaderDetected(header http.Header) bool {
	for _, value := range header.Values("Cf-Mitigated") {
		for _, item := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(item), "challenge") {
				return true
			}
		}
	}
	return false
}

func isHTMLContentType(header http.Header) bool {
	mediaType, _, err := mime.ParseMediaType(header.Get("Content-Type"))
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
	// Ray ID、access-denied 和通用 WAF 标题不是 challenge 证据，避免误触发 solver。
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
