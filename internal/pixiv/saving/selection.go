package saving

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/sdk/download"
)

// Request 保存来源展开后的页选择与 SDK 下载选项；页码为 1-based。
type Request struct {
	download.Options
	Pages []int
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

// ValidateQuality 校验静态图片质量。
func ValidateQuality(quality download.Quality) error {
	switch quality {
	case "", download.QualityOriginal, download.QualityRegular, download.QualitySmall, download.QualityThumb, download.QualityMini:
		return nil
	default:
		return errors.New("quality must be one of original, regular, small, thumb, mini")
	}
}

// ValidateFormat 校验 ugoira 容器格式。
func ValidateFormat(format download.UgoiraFormat) error {
	switch format {
	case "", download.UgoiraFormatGIF, download.UgoiraFormatAPNG:
		return nil
	default:
		return errors.New("ugoira format must be one of gif, apng")
	}
}
