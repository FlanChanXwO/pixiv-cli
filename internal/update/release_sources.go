package update

import (
	_ "embed"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const (
	releaseSourceURLPlaceholder      = "{url}"
	releaseSourceURLQueryPlaceholder = "{url_query}"
)

var releaseSourceIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*$`)

//go:embed release_sources.txt
var embeddedReleaseSources []byte

// releaseSource 是一个版本内置的 GitHub Releases 传输路径。它只改变请求目的地，
// 不改变版本选择、资产名称、签名或 checksum 的信任边界。
type releaseSource struct {
	id    string
	api   releaseSourceTemplate
	asset releaseSourceTemplate
}

type releaseSourceTemplate struct {
	raw         string
	placeholder string
}

func defaultReleaseSources() []releaseSource {
	sources, err := parseReleaseSources(embeddedReleaseSources)
	if err != nil {
		panic(fmt.Sprintf("parse embedded release sources: %v", err))
	}
	return sources
}

func parseReleaseSources(body []byte) ([]releaseSource, error) {
	var sources []releaseSource
	seenIDs := make(map[string]struct{})
	for lineNumber, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "|")
		if len(fields) != 3 {
			return nil, fmt.Errorf("release source line %d must have id, API template, and asset template", lineNumber+1)
		}
		id := strings.TrimSpace(fields[0])
		if !releaseSourceIDPattern.MatchString(id) {
			return nil, fmt.Errorf("release source line %d has invalid ID %q", lineNumber+1, id)
		}
		if _, duplicate := seenIDs[id]; duplicate {
			return nil, fmt.Errorf("release source line %d repeats ID %q", lineNumber+1, id)
		}
		api, err := parseReleaseSourceTemplate(strings.TrimSpace(fields[1]))
		if err != nil {
			return nil, fmt.Errorf("release source line %d API template: %w", lineNumber+1, err)
		}
		asset, err := parseReleaseSourceTemplate(strings.TrimSpace(fields[2]))
		if err != nil {
			return nil, fmt.Errorf("release source line %d asset template: %w", lineNumber+1, err)
		}
		if asset.raw == "" {
			return nil, fmt.Errorf("release source line %d must provide an asset template", lineNumber+1)
		}
		seenIDs[id] = struct{}{}
		sources = append(sources, releaseSource{id: id, api: api, asset: asset})
	}
	if len(sources) == 0 {
		return nil, fmt.Errorf("release source list is empty")
	}
	return sources, nil
}

func parseReleaseSourceTemplate(value string) (releaseSourceTemplate, error) {
	if value == "-" {
		return releaseSourceTemplate{}, nil
	}
	placeholder := ""
	for _, candidate := range []string{releaseSourceURLPlaceholder, releaseSourceURLQueryPlaceholder} {
		count := strings.Count(value, candidate)
		if count == 0 {
			continue
		}
		if count != 1 || placeholder != "" {
			return releaseSourceTemplate{}, fmt.Errorf("must contain exactly one URL placeholder")
		}
		placeholder = candidate
	}
	remaining := strings.Replace(value, placeholder, "", 1)
	if placeholder == "" || strings.Contains(remaining, releaseSourceURLPlaceholder) || strings.Contains(remaining, releaseSourceURLQueryPlaceholder) {
		return releaseSourceTemplate{}, fmt.Errorf("must contain exactly one URL placeholder")
	}
	template := releaseSourceTemplate{raw: value, placeholder: placeholder}
	if _, err := template.apply("https://github.com/FlanChanXwO/pixiv-cli/releases"); err != nil {
		return releaseSourceTemplate{}, err
	}
	return template, nil
}

func (source releaseSource) apiURL(canonical string) (string, error) {
	if source.api.raw == "" {
		return "", fmt.Errorf("release source %q does not support GitHub Releases API", source.id)
	}
	return source.api.apply(canonical)
}

func (source releaseSource) assetURL(canonical string) (string, error) {
	return source.asset.apply(canonical)
}

func (template releaseSourceTemplate) apply(canonical string) (string, error) {
	canonicalURL, err := url.Parse(canonical)
	if err != nil {
		return "", fmt.Errorf("parse canonical release URL: %w", err)
	}
	if canonicalURL.Scheme != "https" || canonicalURL.Host == "" || canonicalURL.User != nil || canonicalURL.Fragment != "" {
		return "", fmt.Errorf("canonical release URL %q must be an absolute HTTPS URL without userinfo or fragment", canonical)
	}
	replacement := canonical
	if template.placeholder == releaseSourceURLQueryPlaceholder {
		replacement = url.QueryEscape(canonical)
	}
	value := strings.Replace(template.raw, template.placeholder, replacement, 1)
	transformed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("parse transformed release URL: %w", err)
	}
	if (transformed.Scheme != "http" && transformed.Scheme != "https") || transformed.Host == "" || transformed.User != nil || transformed.Fragment != "" {
		return "", fmt.Errorf("transformed release URL %q must be an absolute HTTP(S) URL without userinfo or fragment", value)
	}
	return value, nil
}
