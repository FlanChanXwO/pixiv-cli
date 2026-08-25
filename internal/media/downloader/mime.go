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

// MimeTypeForFile 只对已支持的图片格式使用文件签名；无法识别时保留既有
// 扩展名推导语义，避免把普通文本或未知二进制误报为新的媒体类型。
func MimeTypeForFile(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return MimeTypeForPath(path)
	}
	defer file.Close()
	buffer := make([]byte, 512)
	read, readErr := file.Read(buffer)
	if readErr == nil || read > 0 {
		switch mimeType := http.DetectContentType(buffer[:read]); mimeType {
		case "image/jpeg", "image/png", "image/gif", "image/webp":
			return mimeType
		}
	}
	return MimeTypeForPath(path)
}
