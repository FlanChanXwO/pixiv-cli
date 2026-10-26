// 下载请求的取值类型、页码解析与质量/容器校验。

package downloader

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// DownloadQuality 选择静态图片的下载分辨率。
type DownloadQuality string

const (
	DownloadQualityOriginal DownloadQuality = "original"
	DownloadQualityRegular  DownloadQuality = "regular"
	DownloadQualitySmall    DownloadQuality = "small"
	DownloadQualityThumb    DownloadQuality = "thumb"
	DownloadQualityMini     DownloadQuality = "mini"
)

// UgoiraFormat 选择 ugoira 的最终动画容器。
type UgoiraFormat string

const (
	UgoiraFormatGIF  UgoiraFormat = "gif"
	UgoiraFormatAPNG UgoiraFormat = "apng"
)

// DownloadRequest 描述一次下载：作品 ID、目标目录、命名模板与页/质量选择。
// DirectoryTemplate 为空时沿用既有的扁平/多页目录布局。
type DownloadRequest struct {
	IllustIDs         []int64
	DownloadPath      string
	FilenameTemplate  string
	DirectoryTemplate string
	// Pages 为 1-based 页码列表；空表示全部页。由 ParsePageSpec 生成。
	Pages []int
	// Quality 默认 original。
	Quality DownloadQuality
	// UgoiraFormat 默认 GIF，仅影响 ugoira 的最终动画容器。
	UgoiraFormat UgoiraFormat
}

// ParsePageSpec 解析逗号/连字符分隔的 1-based 页码选择，返回闭区间页号列表。
// 单个范围在读取作品元数据前就会被展开，因此这里拒绝跨度过大的范围，避免
// 1-1000000000 这类无界输入立即分配巨量内存或在 page++ 溢出时死循环。
func ParsePageSpec(raw string) ([]int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var pages []int
	seen := make(map[int]struct{})
	for part := range strings.SplitSeq(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, errors.New("page selection contains an empty entry")
		}
		var start, end int
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			parsedStart, err := strconv.Atoi(strings.TrimSpace(bounds[0]))
			if err != nil || parsedStart <= 0 {
				return nil, fmt.Errorf("invalid page range %q", part)
			}
			parsedEnd, err := strconv.Atoi(strings.TrimSpace(bounds[1]))
			if err != nil || parsedEnd < parsedStart {
				return nil, fmt.Errorf("invalid page range %q", part)
			}
			if parsedEnd-parsedStart > maxPageExpansion {
				return nil, fmt.Errorf("page range %q is too large", part)
			}
			start, end = parsedStart, parsedEnd
		} else {
			page, err := strconv.Atoi(part)
			if err != nil || page <= 0 {
				return nil, fmt.Errorf("invalid page number %q", part)
			}
			start, end = page, page
		}
		// 按长度迭代，避免最大 int 页号在末次递增时溢出。
		for offset := range end - start + 1 {
			page := start + offset
			if _, exists := seen[page]; exists {
				continue
			}
			seen[page] = struct{}{}
			pages = append(pages, page)
		}
	}
	slices.Sort(pages)
	return pages, nil
}

// maxPageExpansion 限制单个范围展开的页数。真实的 Pixiv 作品页数远小于此值，
// 超过即视为用户误传的无界范围并在解析期拒绝。
const maxPageExpansion = 100000

// ValidateDownloadQuality 校验静态图片质量。
func ValidateDownloadQuality(quality DownloadQuality) error {
	switch quality {
	case "", DownloadQualityOriginal, DownloadQualityRegular, DownloadQualitySmall, DownloadQualityThumb, DownloadQualityMini:
		return nil
	default:
		return errors.New("quality must be one of original, regular, small, thumb, mini")
	}
}

// ValidateUgoiraFormat 校验 ugoira 容器格式。
func ValidateUgoiraFormat(format UgoiraFormat) error {
	switch format {
	case "", UgoiraFormatGIF, UgoiraFormatAPNG:
		return nil
	default:
		return errors.New("ugoira format must be one of gif, apng")
	}
}
