package downloader

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// DirectResourceClient 定义直链 source 使用的窄端口；作品元数据下载仍只依赖 DownloadClient。
type DirectResourceClient interface {
	SaveResourceURL(context.Context, string, sdk.SaveOptions) (sdk.SavedResource, error)
}

// DownloadTargetClient 在下载资源能力之外提供作者作品列表，用于把用户 URL 展开为视觉作品。
type DownloadTargetClient interface {
	DownloadClient
	UserArtworks(context.Context, pixiv.UserArtworksRequest) (sdk.Page[pixiv.Artwork], error)
	UserArtworkBookmarks(context.Context, pixiv.UserArtworkBookmarksRequest) (sdk.Page[pixiv.Artwork], error)
}

func directResourcePath(downloadPath string, ref sdk.ResourceRef) (string, error) {
	if strings.TrimSpace(downloadPath) == "" {
		return "", errors.New("download path is required for direct resource sources")
	}
	// 直接 resource 源没有作品标题或页号，必须用完整 ref 的稳定摘要作为文件名。
	// 之前截断 ref 前 64 字符会让同 kind 不同 ID/page 的 ref 共享前缀并互相覆盖。
	name := ref.String()
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

type redactedResourceError struct {
	message string
	cause   error
}

func (e redactedResourceError) Error() string { return e.message }
func (e redactedResourceError) Unwrap() error { return e.cause }

func redactDirectResourceError(rawURL string, err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	redacted := message
	if rawURL != "" {
		redacted = strings.ReplaceAll(redacted, rawURL, redactedDownloadSource)
		if parsed, parseErr := url.Parse(rawURL); parseErr == nil {
			if parsed.RawQuery != "" {
				redacted = strings.ReplaceAll(redacted, parsed.RawQuery, "[redacted]")
			}
			for key := range parsed.Query() {
				redacted = redactDirectResourceQueryParameter(redacted, key)
			}
		}
	}
	if redacted == message {
		return err
	}
	return redactedResourceError{message: redacted, cause: err}
}

func redactDirectResourceQueryParameter(message, key string) string {
	if key == "" {
		return message
	}
	marker := key + "="
	searchFrom := 0
	for searchFrom < len(message) {
		relativeIndex := strings.Index(message[searchFrom:], marker)
		if relativeIndex < 0 {
			return message
		}
		index := searchFrom + relativeIndex
		valueStart := index + len(marker)
		valueEnd := strings.IndexAny(message[valueStart:], "& \t\r\n\"'<>")
		if valueEnd < 0 {
			valueEnd = len(message)
		} else {
			valueEnd += valueStart
		}
		message = message[:valueStart] + "[redacted]" + message[valueEnd:]
		searchFrom = valueStart + len("[redacted]")
	}
	return message
}
