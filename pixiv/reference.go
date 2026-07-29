package pixiv

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

// ReferenceKind 标识稳定 Pixiv 页面 URL 指向的资源类型。
type ReferenceKind string

const (
	ReferenceKindArtwork ReferenceKind = "artwork"
	ReferenceKindUser    ReferenceKind = "user"
)

// Reference 是本地解析后的 Pixiv 稳定资源标识，不包含原始 URL 或查询参数。
type Reference struct {
	Kind ReferenceKind
	ID   int64
}

// URL 返回资源的稳定规范 Pixiv 页面地址；非法零值不生成虚假的 URL。
func (r Reference) URL() string {
	if r.ID <= 0 {
		return ""
	}
	switch r.Kind {
	case ReferenceKindArtwork:
		return "https://www.pixiv.net/artworks/" + strconv.FormatInt(r.ID, 10)
	case ReferenceKindUser:
		return "https://www.pixiv.net/users/" + strconv.FormatInt(r.ID, 10)
	default:
		return ""
	}
}

var errUnsupportedReference = errors.New("reference must be a positive artwork ID or a supported Pixiv URL")

// ParseReference 只解析稳定的 Pixiv 作品页、用户主页和用户作品页 URL，或正整数作品 ID。
// 它不会请求网络、跟随重定向或保留原始输入，避免把 query 中可能存在的敏感信息带进错误和日志。
func ParseReference(raw string) (Reference, error) {
	value := strings.TrimSpace(raw)
	if id, ok := parsePositiveReferenceID(value); ok {
		return Reference{Kind: ReferenceKindArtwork, ID: id}, nil
	}

	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Port() != "" || !isPixivHost(parsed.Hostname()) {
		return Reference{}, errUnsupportedReference
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if reference, ok := parseReferencePath(parts); ok {
		return reference, nil
	}
	if len(parts) >= 3 && isLocaleSegment(parts[0]) {
		if reference, ok := parseReferencePath(parts[1:]); ok {
			return reference, nil
		}
	}
	return Reference{}, errUnsupportedReference
}

// parseReferencePath 先匹配无 locale 的稳定路径，避免 users/artworks 这类合法路径段
// 被宽松的 locale 语法错误地吞掉。
func parseReferencePath(parts []string) (Reference, bool) {
	if len(parts) == 2 && parts[0] == "artworks" {
		if id, ok := parsePositiveReferenceID(parts[1]); ok {
			return Reference{Kind: ReferenceKindArtwork, ID: id}, true
		}
	}
	if len(parts) == 2 && parts[0] == "users" {
		if id, ok := parsePositiveReferenceID(parts[1]); ok {
			return Reference{Kind: ReferenceKindUser, ID: id}, true
		}
	}
	if len(parts) == 3 && parts[0] == "users" && parts[2] == "artworks" {
		if id, ok := parsePositiveReferenceID(parts[1]); ok {
			return Reference{Kind: ReferenceKindUser, ID: id}, true
		}
	}
	return Reference{}, false
}

// ParseArtworkReference 解析单件作品引用；用户 URL 和其他 Pixiv 页面不在 detail 的输入范围内。
func ParseArtworkReference(raw string) (int64, error) {
	reference, err := ParseReference(raw)
	if err != nil || reference.Kind != ReferenceKindArtwork {
		return 0, errUnsupportedReference
	}
	return reference.ID, nil
}

// ParseUserReference 解析单个 Pixiv 用户 ID、用户主页或用户作品页 URL。纯数字在
// ParseReference 中默认是作品 ID；此处作为用户领域专用入口按用户 ID 解释。
func ParseUserReference(raw string) (int64, error) {
	if id, ok := parsePositiveReferenceID(strings.TrimSpace(raw)); ok {
		return id, nil
	}
	reference, err := ParseReference(raw)
	if err != nil || reference.Kind != ReferenceKindUser {
		return 0, errUnsupportedReference
	}
	return reference.ID, nil
}

func parsePositiveReferenceID(value string) (int64, bool) {
	id, err := strconv.ParseInt(value, 10, 64)
	return id, err == nil && id > 0
}

func isPixivHost(host string) bool {
	switch strings.ToLower(host) {
	case "pixiv.net", "www.pixiv.net":
		return true
	default:
		return false
	}
}

// isLocaleSegment 只放行 URL 路径中常见的 BCP 47 风格 locale，避免把任意层级误判为资源 URL。
func isLocaleSegment(value string) bool {
	parts := strings.Split(value, "-")
	if len(parts) == 0 || len(parts) > 2 {
		return false
	}
	for _, part := range parts {
		if len(part) < 2 || len(part) > 8 {
			return false
		}
		for _, char := range part {
			if !unicode.IsLetter(char) {
				return false
			}
		}
	}
	return true
}
