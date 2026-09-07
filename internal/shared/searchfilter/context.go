// Package searchfilter 定义 CLI/MCP 共用的搜索筛选 continuation 语义。
package searchfilter

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
)

// BookmarkContext 绑定实际采用的收藏筛选策略；nil 与显式零值语义不同。
// 调用方保留产品筛选，SDK 只绑定摘要，不把本地条件发送到 upstream。
func BookmarkContext(min, max *int, strategy string) string {
	bound := func(value *int) string {
		if value == nil {
			return "none"
		}
		return strconv.Itoa(*value)
	}
	sum := sha256.Sum256([]byte("bookmark/v1\n" + strategy + "\n" + bound(min) + "\n" + bound(max)))
	return hex.EncodeToString(sum[:])
}
