package downloader

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func MimeTypeForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}

// MimeTypeForFile 优先使用已支持的文件签名；无法识别时保留 URL/路径扩展名
// 兜底，供 CLI/MCP 展示层在上游没有返回 Content-Type 时继续工作。
func MimeTypeForFile(path string) string {
	if mimeType := detectImageMimeType(path); mimeType != "" {
		return mimeType
	}
	return MimeTypeForPath(path)
}

// detectImageMimeType 只返回受支持图片的签名结果；它不会把路径扩展名
// 当作签名，从而让下载发布逻辑能够区分“已确认格式”和“最后兜底”。
func detectImageMimeType(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	buffer := make([]byte, 512)
	read, _ := file.Read(buffer)
	if read <= 0 {
		return ""
	}
	mediaType := normalizeMediaType(http.DetectContentType(buffer[:read]))
	if _, ok := imageExtensionForMediaType(mediaType); !ok {
		return ""
	}
	return mediaType
}

func normalizeMediaType(contentType string) string {
	return strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
}

func imageExtensionForMediaType(mediaType string) (string, bool) {
	switch normalizeMediaType(mediaType) {
	case "image/jpeg":
		return ".jpg", true
	case "image/png":
		return ".png", true
	case "image/gif":
		return ".gif", true
	case "image/webp":
		return ".webp", true
	default:
		return "", false
	}
}
