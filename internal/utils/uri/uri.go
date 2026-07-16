package uri

import (
	"errors"
	"net/url"
	"path/filepath"
)

// ErrInvalidProxy 是 HTTP client 构造链共享的代理配置分类契约。
// 调用方只能包装静态上下文，不能把可能含凭据的原始 URL 或解析错误放入错误链。
var ErrInvalidProxy = errors.New("invalid proxy configuration")

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
