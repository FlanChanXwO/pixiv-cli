package saving

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/media/filename"
)

func directResourcePath(downloadPath string, ref resourceReference) (string, error) {
	if strings.TrimSpace(downloadPath) == "" {
		return "", errors.New("download path is required for direct resource sources")
	}
	// 直接 resource 源没有作品标题或页号，必须用完整 ref 的稳定摘要作为文件名。
	// 之前截断 ref 前 64 字符会让同 kind 不同 ID/page 的 ref 共享前缀并互相覆盖。
	name := ref.Text
	digest := sha256.Sum256([]byte(name))
	encoded := hex.EncodeToString(digest[:])
	return filepath.Join(downloadPath, "resource-"+encoded), nil
}

func isDirectResourceURL(source string) bool {
	trimmed := strings.TrimSpace(source)
	lower := strings.ToLower(trimmed)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func directResourcePathFromURL(downloadPath, rawURL string) (string, error) {
	if strings.TrimSpace(downloadPath) == "" {
		return "", errors.New("download path is required for direct resource sources")
	}
	name, err := directResourceBasename(rawURL)
	if err != nil {
		return "", err
	}
	return filepath.Join(downloadPath, name), nil
}

func directResourceBasename(rawURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", errors.New("direct resource URL is invalid")
	}
	escapedPath := parsed.EscapedPath()
	if escapedPath == "" || escapedPath == "/" || strings.HasSuffix(escapedPath, "/") {
		return "", errors.New("direct resource URL has no usable basename")
	}
	name, err := url.PathUnescape(path.Base(escapedPath))
	if err != nil || name == "" || name == "." || name == ".." {
		return "", errors.New("direct resource URL has no usable basename")
	}
	name = sanitizeResourceName(name)
	if name == "" || name == "." || name == ".." {
		return "", errors.New("direct resource URL has no usable basename")
	}
	// 直链可能共享可读 basename；将完整解析 URL 的摘要追加到扩展名前，
	// 让不同路径或 query 的资源不会静默写入同一个目标。签名 query 只参与
	// 摘要计算，不会原样进入文件名。
	digest := sha256.Sum256([]byte(parsed.String()))
	suffix := hex.EncodeToString(digest[:6])
	ext := path.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	if stem == "" {
		stem = name
		ext = ""
	}
	return stem + "-" + suffix + ext, nil
}

// sanitizeResourceName 统一 URL basename 与扩展名的跨平台字符规则。
func sanitizeResourceName(name string) string {
	name = strings.Map(func(character rune) rune {
		if character < 0x20 || character == 0x7f {
			return '_'
		}
		return character
	}, filename.Sanitize(name))
	return strings.TrimRight(name, ". ")
}
