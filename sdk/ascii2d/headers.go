package ascii2d

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

const (
	headerUpgrade          = "Upgrade-Insecure-Requests"
	headerUserAgent        = "User-Agent"
	headerAccept           = "Accept"
	headerReferer          = "Referer"
	headerOrigin           = "Origin"
	headerFetchSite        = "Sec-Fetch-Site"
	headerFetchMode        = "Sec-Fetch-Mode"
	headerFetchUser        = "Sec-Fetch-User"
	headerFetchDestination = "Sec-Fetch-Dest"
	headerEncoding         = "Accept-Encoding"
	headerLanguage         = "Accept-Language"
	headerClientAgent      = "Sec-CH-UA"
	headerClientMobile     = "Sec-CH-UA-Mobile"
	headerClientPlatform   = "Sec-CH-UA-Platform"
	fetchSameOrigin        = "same-origin"
	fetchNavigate          = "navigate"
	fetchDocument          = "document"
	// Keep the default agent aligned with the shared Chrome_146 TLS profile.
	defaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"
	browserAccept    = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"
	browserEncoding  = "gzip, deflate, br"
	browserLanguage  = "en-US,en;q=0.9"
)

var chromiumUserAgentPatterns = []struct {
	pattern *regexp.Regexp
	brand   string
}{
	{pattern: regexp.MustCompile(`(?:^|[\s(;])EdgiOS/([0-9]+)`), brand: "Microsoft Edge"},
	{pattern: regexp.MustCompile(`(?:^|[\s(;])EdgA/([0-9]+)`), brand: "Microsoft Edge"},
	{pattern: regexp.MustCompile(`(?:^|[\s(;])Edg/([0-9]+)`), brand: "Microsoft Edge"},
	{pattern: regexp.MustCompile(`(?:^|[\s(;])OPR/([0-9]+)`), brand: "Opera"},
	{pattern: regexp.MustCompile(`(?:^|[\s(;])CriOS/([0-9]+)`), brand: "Google Chrome"},
	{pattern: regexp.MustCompile(`(?:^|[\s(;])Chrome/([0-9]+)`), brand: "Google Chrome"},
	{pattern: regexp.MustCompile(`(?:^|[\s(;])Chromium/([0-9]+)`), brand: "Chromium"},
}

// clientHints omits Chromium hints for non-Chromium agents.
type clientHints struct {
	enabled  bool
	value    string
	mobile   string
	platform string
}

func parseClientHints(userAgent string) clientHints {
	var (
		version string
		brand   string
	)
	for _, candidate := range chromiumUserAgentPatterns {
		matches := candidate.pattern.FindStringSubmatch(userAgent)
		if len(matches) == 2 {
			version = matches[1]
			brand = candidate.brand
			break
		}
	}
	if version == "" {
		return clientHints{}
	}

	platform := userAgentPlatform(userAgent)
	mobile := "?0"
	if platform == "Android" || platform == "iOS" {
		mobile = "?1"
	}
	brands := []string{
		`"Not(A:Brand";v="24"`,
		fmt.Sprintf(`"Chromium";v="%s"`, version),
	}
	if brand != "Chromium" {
		brands = append(brands, fmt.Sprintf(`"%s";v="%s"`, brand, version))
	}
	return clientHints{
		enabled:  true,
		value:    strings.Join(brands, ", "),
		mobile:   mobile,
		platform: platform,
	}
}

func userAgentPlatform(userAgent string) string {
	switch {
	case strings.Contains(userAgent, "Android"):
		return "Android"
	case strings.Contains(userAgent, "iPhone"), strings.Contains(userAgent, "iPad"), strings.Contains(userAgent, "iPod"):
		return "iOS"
	case strings.Contains(userAgent, "Windows"):
		return "Windows"
	case strings.Contains(userAgent, "Macintosh"):
		return "macOS"
	case strings.Contains(userAgent, "Linux"):
		return "Linux"
	default:
		return ""
	}
}

func (c *searchSession) applyNavigationHeaders(request *http.Request, fetchSite, referer string) {
	header := make(http.Header)
	c.applyClientHints(header)
	header.Set(headerUpgrade, "1")
	header.Set(headerUserAgent, c.userAgent)
	header.Set(headerAccept, browserAccept)
	if referer != "" {
		header.Set(headerReferer, referer)
	}
	header.Set(headerFetchSite, fetchSite)
	header.Set(headerFetchMode, fetchNavigate)
	header.Set(headerFetchUser, "?1")
	header.Set(headerFetchDestination, fetchDocument)
	header.Set(headerEncoding, browserEncoding)
	header.Set(headerLanguage, browserLanguage)
	request.Header = header
}

func (c *searchSession) applyFormHeaders(request *http.Request) {
	c.applyNavigationHeaders(request, fetchSameOrigin, homeURL)
	request.Header.Del(headerUpgrade)
	request.Header.Set(headerOrigin, origin)
}

func (c *searchSession) applyClientHints(header http.Header) {
	if !c.hints.enabled {
		return
	}
	header.Set(headerClientAgent, c.hints.value)
	header.Set(headerClientMobile, c.hints.mobile)
	if c.hints.platform != "" {
		header.Set(headerClientPlatform, fmt.Sprintf(`"%s"`, c.hints.platform))
	}
}
