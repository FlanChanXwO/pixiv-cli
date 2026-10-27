package downloader

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/media/filename"
)

// downloadExtension 清理 URL path 推导出的扩展名，避免跨平台非法文件名字符
// 绕过作品标题和模板已有的文件名规范化边界。ASCII C0/DEL 控制字符
// 不适合作为文件名内容，Windows 还不接受尾随点或空格，因而在扩展名边界统一处理。
func downloadExtension(rawURL string) string {
	return sanitizeResourceName(filepath.Ext(pathFromURL(rawURL)))
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

// publishDetectedImageExtension 统一修正静态质量 URL 后缀与实际实体格式不一致的情况。
// 响应 MIME 优先，文件签名次之，只有上游没有返回 MIME 时才使用 URL 后缀兜底。
// hard link 发布不会覆盖同名目标；删除旧路径失败时撤销新 link，保留原文件并暴露错误。
func publishDetectedImageExtension(path, contentType string) (string, error) {
	reportedType := normalizeMediaType(contentType)
	mediaType := reportedType
	extension, ok := imageExtensionForMediaType(mediaType)
	if !ok {
		mediaType = detectImageMimeType(path)
		extension, ok = imageExtensionForMediaType(mediaType)
	}
	if !ok && reportedType == "" {
		mediaType = mimeTypeForPath(path)
		extension, ok = imageExtensionForMediaType(mediaType)
	}
	if !ok {
		if reportedType == "" {
			reportedType = mediaType
		}
		return path, fmt.Errorf("unsupported image content type %q", reportedType)
	}

	current := filepath.Ext(path)
	if strings.EqualFold(current, extension) || (extension == ".jpg" && strings.EqualFold(current, ".jpeg")) {
		return path, nil
	}
	target := strings.TrimSuffix(path, current) + extension
	if err := os.Link(path, target); err != nil {
		return "", fmt.Errorf("publish detected image extension: %w", err)
	}
	if err := os.Remove(path); err != nil {
		return "", errors.Join(fmt.Errorf("remove mismatched image path: %w", err), os.Remove(target))
	}
	return target, nil
}

// MimeTypeForFile 优先使用已支持的文件签名；无法识别时保留 URL/路径扩展名
// 兜底，供 CLI/MCP 展示层在上游没有返回 Content-Type 时继续工作。
func MimeTypeForFile(path string) string {
	if mimeType := detectImageMimeType(path); mimeType != "" {
		return mimeType
	}
	return mimeTypeForPath(path)
}

func mimeTypeForPath(path string) string {
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
