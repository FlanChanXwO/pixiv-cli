// Package continuation 解析 App API 响应中的 endpoint continuation。
//
// continuation 只提取已冻结的 typed value，不把 next_url 原文带出 endpoint
// 边界。调用方必须为每个 operation 提供准确的路径、唯一 continuation key
// 和允许出现的 base query keys。
package continuation

import (
	"net/url"
	"strconv"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
)

const appAPIHost = "app-api.pixiv.net"

// Spec 描述一个 endpoint 的 continuation allowlist。
type Spec struct {
	Path             string
	Keys             []string
	AllowZero        bool
	AllowedQueryKeys []string
}

// Parse 校验并提取 rawURL 中的唯一 typed continuation value。
//
// next_url 必须指向固定的 App API HTTPS host 和当前 operation path。查询中
// 允许出现当前 operation 的 base query keys，但每个 key 只能出现一次，且
// 未知 key、缺失/重复 continuation key 和越界值都会被视为 malformed。
func Parse(rawURL string, spec Spec) (string, int64, error) {
	if rawURL == "" || spec.Path == "" || len(spec.Keys) == 0 {
		return "", 0, protocol.MalformedResponse()
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Fragment != "" || parsed.Opaque != "" || parsed.Hostname() != appAPIHost || parsed.Port() != "" || parsed.Path != spec.Path || parsed.EscapedPath() != spec.Path {
		return "", 0, protocol.MalformedResponse()
	}
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return "", 0, protocol.MalformedResponse()
	}
	allowed := make(map[string]struct{}, len(spec.AllowedQueryKeys)+len(spec.Keys))
	for _, key := range spec.Keys {
		if key == "" {
			return "", 0, protocol.MalformedResponse()
		}
		allowed[key] = struct{}{}
	}
	for _, key := range spec.AllowedQueryKeys {
		allowed[key] = struct{}{}
	}
	for key, entries := range values {
		if _, ok := allowed[key]; !ok || len(entries) != 1 {
			return "", 0, protocol.MalformedResponse()
		}
	}
	var continuationKey string
	for _, key := range spec.Keys {
		if _, ok := values[key]; !ok {
			continue
		}
		if continuationKey != "" {
			return "", 0, protocol.MalformedResponse()
		}
		continuationKey = key
	}
	if continuationKey == "" {
		return "", 0, protocol.MalformedResponse()
	}
	entries := values[continuationKey]
	value, err := strconv.ParseInt(entries[0], 10, 64)
	if err != nil || value < 0 || (!spec.AllowZero && value == 0) {
		return "", 0, protocol.MalformedResponse()
	}
	return continuationKey, value, nil
}
