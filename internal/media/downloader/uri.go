// 本地路径与 file URI 的转换，以及 URL path 提取。

package downloader

import (
	"net/url"
	"path/filepath"
)

// FileURI 把本地路径转成绝对 file URI，供 MCP 结构化输出引用已下载文件。
func FileURI(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

func pathFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Path == "" || (parsed.Scheme == "" && parsed.Host == "") {
		return rawURL
	}
	return parsed.Path
}
