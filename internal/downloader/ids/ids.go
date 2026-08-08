// Package ids 提供本地请求参数使用的 Pixiv 标识归一化工具。
package ids

import "slices"

// DeduplicatePositive 去除非法和重复 ID，并以稳定升序交给下载计划。
func DeduplicatePositive(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	unique := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	slices.Sort(unique)
	return unique
}
