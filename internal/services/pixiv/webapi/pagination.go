package webapi

import "errors"

const (
	// 两个值是对应 Pixiv Web endpoint 的固定 wire batch 边界；边界测试
	// 分别以 59/60 与 49/50 锁定换页行为，不是本地结果条数限制。
	artworkSearchPageSize = 60
	illustRankingPageSize = 50
)

// ErrUnrepresentablePagination 标识 cursor offset 无法安全换算为 Web 页码和下一页边界。
var ErrUnrepresentablePagination = errors.New("web api pagination cannot represent cursor offset")

type webPagination struct {
	page       int
	nextOffset int
}

func checkedWebPagination(offset, pageSize int) (webPagination, error) {
	if offset < 0 || pageSize <= 0 {
		return webPagination{}, ErrUnrepresentablePagination
	}
	quotient := offset / pageSize
	maxInt := int(^uint(0) >> 1)
	if quotient == maxInt {
		return webPagination{}, ErrUnrepresentablePagination
	}
	page := quotient + 1
	if page > maxInt/pageSize {
		return webPagination{}, ErrUnrepresentablePagination
	}
	// Pixiv Web 使用固定批次页码；同时预检下一页边界，避免响应后产生负 cursor。
	return webPagination{page: page, nextOffset: page * pageSize}, nil
}

func trimWebPageOffset[T any](items []T, offset, pageSize int) []T {
	if offset <= 0 || pageSize <= 0 {
		return items
	}
	skip := offset % pageSize
	if skip >= len(items) {
		return nil
	}
	return items[skip:]
}

func webHasNext(offset, rawCount int, total int64, pageSize int) bool {
	if rawCount == 0 {
		return false
	}
	if total > 0 {
		pageStart := int64(offset - offset%pageSize)
		if pageStart >= total {
			return false
		}
		// 用减法表达 pageStart+rawCount<total，避免靠近 MaxInt 时加法回绕。
		return int64(rawCount) < total-pageStart
	}
	// 无 total 时，只在完整上游批次后继续；下一空批次会自然收敛。
	return rawCount >= pageSize
}
