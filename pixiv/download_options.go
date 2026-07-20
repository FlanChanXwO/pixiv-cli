package pixiv

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// DownloadQuality 是静态图派生质量；语义固定，不静默降级。
// original=原图，regular=最长边约 1200，small=最长边约 540，
// thumb=居中裁剪约 250x250，mini=居中裁剪约 48x48。
type DownloadQuality string

const (
	DownloadQualityOriginal DownloadQuality = "original"
	DownloadQualityRegular  DownloadQuality = "regular"
	DownloadQualitySmall    DownloadQuality = "small"
	DownloadQualityThumb    DownloadQuality = "thumb"
	DownloadQualityMini     DownloadQuality = "mini"
)

// DownloadOptions 是作品下载的页面与质量选择。
// Pages 为空表示全部页；页码为 1-based。
type DownloadOptions struct {
	Pages   []int
	Quality DownloadQuality
}

// ParsePageSpec 解析 "1,3-5" 形式的 1-based 闭区间页码；去重并按自然页序排序。
// 空串表示未指定页选择（下载全部）。
func ParsePageSpec(spec string) ([]int, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	seen := make(map[int]struct{})
	var pages []int
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("page selection contains an empty item")
		}
		if strings.Contains(part, "-") {
			bounds := strings.Split(part, "-")
			if len(bounds) != 2 {
				return nil, fmt.Errorf("invalid page range %q", part)
			}
			start, err := strconv.Atoi(strings.TrimSpace(bounds[0]))
			if err != nil {
				return nil, fmt.Errorf("invalid page range start %q", bounds[0])
			}
			end, err := strconv.Atoi(strings.TrimSpace(bounds[1]))
			if err != nil {
				return nil, fmt.Errorf("invalid page range end %q", bounds[1])
			}
			if start <= 0 || end <= 0 {
				return nil, fmt.Errorf("page numbers must be positive (1-based)")
			}
			if end < start {
				return nil, fmt.Errorf("page range %q has end before start", part)
			}
			for page := start; page <= end; page++ {
				if _, ok := seen[page]; ok {
					continue
				}
				seen[page] = struct{}{}
				pages = append(pages, page)
			}
			continue
		}
		page, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid page number %q", part)
		}
		if page <= 0 {
			return nil, fmt.Errorf("page numbers must be positive (1-based)")
		}
		if _, ok := seen[page]; ok {
			continue
		}
		seen[page] = struct{}{}
		pages = append(pages, page)
	}
	sort.Ints(pages)
	return pages, nil
}

// ValidateDownloadQuality 校验质量枚举。
func ValidateDownloadQuality(quality DownloadQuality) error {
	switch quality {
	case DownloadQualityOriginal, DownloadQualityRegular, DownloadQualitySmall, DownloadQualityThumb, DownloadQualityMini:
		return nil
	default:
		return fmt.Errorf("quality must be one of original, regular, small, thumb, mini")
	}
}
