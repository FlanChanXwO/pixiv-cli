package uri

import (
	"net/url"
	"path/filepath"
)

func PathFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Path == "" || (parsed.Scheme == "" && parsed.Host == "") {
		return rawURL
	}
	return parsed.Path
}

func FileURI(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}
