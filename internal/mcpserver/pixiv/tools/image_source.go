package tools

import (
	"net/url"
	"path/filepath"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/reversesearch"
)

// imageSource 共用反向搜图与向量搜索的本机来源及宿主图片校验。
func imageSource(sourceInput *string, imageInput *reverseSearchImage) (string, reversesearch.ErrorCode) {
	if (sourceInput == nil) == (imageInput == nil) {
		return "", reversesearch.CodeInvalidRequest
	}
	var source string
	if sourceInput != nil {
		source = *sourceInput
		if strings.HasPrefix(strings.ToLower(source), "file://") {
			parsed, err := url.Parse(source)
			// file URI 只接受 server 本机绝对路径；带 authority、查询或片段时直接拒绝。
			if err != nil || parsed.User != nil || (parsed.Host != "" && !strings.EqualFold(parsed.Host, "localhost")) || parsed.RawQuery != "" || parsed.ForceQuery || strings.Contains(source, "#") || strings.ContainsRune(parsed.Path, 0) {
				return "", reversesearch.CodeInvalidSource
			}
			source = filepath.FromSlash(parsed.Path)
			// Windows 的 file:///C:/... 多出一个前导分隔符，需要去掉；其他平台保持原样。
			if len(source) > 0 && filepath.VolumeName(source[1:]) != "" {
				source = source[1:]
			}
			if !filepath.IsAbs(source) || strings.HasPrefix(filepath.ToSlash(source), "//") {
				return "", reversesearch.CodeInvalidSource
			}
		}
	} else {
		source = imageInput.DownloadURL
		parsed, err := url.Parse(source)
		if strings.TrimSpace(imageInput.FileID) == "" || err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return "", reversesearch.CodeInvalidRequest
		}
	}
	if strings.TrimSpace(source) == "" {
		return "", reversesearch.CodeInvalidRequest
	}
	return source, ""
}
