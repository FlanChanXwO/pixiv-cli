package pixiv

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultDownloadPath 是快捷下载 API 使用的稳定默认目录。
	DefaultDownloadPath = "./downloads"
	// DefaultFilenameTemplate 是作品下载的稳定默认命名模板。
	DefaultFilenameTemplate = "{author} - {title}_{id}"
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

// UgoiraMode 是 ugoira 最终产物的模式。零值按 GIF 处理。
type UgoiraMode string

const (
	UgoiraModeGIF    UgoiraMode = "gif"
	UgoiraModeAPNG   UgoiraMode = "apng"
	UgoiraModeZIP    UgoiraMode = "zip"
	UgoiraModeFrames UgoiraMode = "frames"
)

// UgoiraFormat 是兼容旧 SDK 调用方的别名；CLI 不再提供 --ugoira-format。
type UgoiraFormat = UgoiraMode

const (
	UgoiraFormatGIF  = UgoiraModeGIF
	UgoiraFormatAPNG = UgoiraModeAPNG
)

// RetryPolicy 控制单个资源请求的显式重试。nil 策略使用默认 3 次重试与 1 秒初始
// 间隔；Retries=0 可明确关闭重试，避免把零值误解为两种不同的用户意图。
type RetryPolicy struct {
	Retries      int
	InitialDelay time.Duration
}

// DownloadOptions 控制高级下载 API 的输出与作品选择。
// DownloadPath 为空时使用 DefaultDownloadPath；FilenameTemplate 为空时使用
// DefaultFilenameTemplate。Pages 为空表示全部页；页码为 1-based。Concurrency 为
// 零时由 SDK 根据当前 GOMAXPROCS 自动决定。CDN 直链只有 DownloadPath 和自动/显式
// Concurrency 有意义；页选择、派生质量和自定义作品模板会明确失败。
type DownloadOptions struct {
	DownloadPath      string
	FilenameTemplate  string
	DirectoryTemplate string
	// Pages 保留闭区间 SDK 兼容接口；不能与 PageSelection 同时使用。
	Pages         []int
	PageSelection *PageSelection
	Quality       DownloadQuality
	UgoiraMode    UgoiraMode
	// UgoiraFormat 是旧 SDK 字段别名；UgoiraMode 非空时优先。
	UgoiraFormat  UgoiraFormat
	Concurrency   int
	Filter        *IllustFilter
	ArchivePath   string
	WriteMetadata bool
	RetryPolicy   *RetryPolicy
	// Progress 是纯观察回调。每个下载 worker 会直接、并发地调用它；回调必须自行
	// 保证并发安全，并且不得阻塞。通过取消传入 DownloadAllWith 的 context 停止下载。
	Progress func(DownloadProgress)
}

// PageSelection 保留用户给出的页选择，支持 "3-" 这种必须在取得作品页数后才能
// 展开的开区间。闭区间仍可通过 ParsePageSpec 获得兼容的 []int。
type PageSelection struct {
	pages    []int
	openFrom int
}

// ParsePageSelection 解析 1-based 页选择，支持逗号、闭区间和一个开区间，例如
// "1,3-5,8-"。空串表示全部页；页码排序并去重。
func ParsePageSelection(spec string) (*PageSelection, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	selection := &PageSelection{}
	seen := make(map[int]struct{})
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("page selection contains an empty item")
		}
		if strings.HasSuffix(part, "-") {
			if selection.openFrom != 0 || strings.Count(part, "-") != 1 {
				return nil, fmt.Errorf("invalid open page range %q", part)
			}
			start, err := strconv.Atoi(strings.TrimSuffix(part, "-"))
			if err != nil || start <= 0 {
				return nil, fmt.Errorf("invalid page range start %q", strings.TrimSuffix(part, "-"))
			}
			selection.openFrom = start
			continue
		}
		pages, err := ParsePageSpec(part)
		if err != nil {
			return nil, err
		}
		for _, page := range pages {
			if _, ok := seen[page]; ok {
				continue
			}
			seen[page] = struct{}{}
			selection.pages = append(selection.pages, page)
		}
	}
	sort.Ints(selection.pages)
	return selection, nil
}

// Resolve 按实际总页数展开 PageSelection。total 必须是作品实际页数。
func (s *PageSelection) Resolve(total int) ([]int, error) {
	if s == nil {
		return nil, nil
	}
	if total <= 0 {
		return nil, fmt.Errorf("page count must be positive")
	}
	seen := make(map[int]struct{}, len(s.pages)+total)
	for _, page := range s.pages {
		if page < 1 || page > total {
			return nil, fmt.Errorf("page %d does not exist (page_count=%d)", page, total)
		}
		seen[page] = struct{}{}
	}
	if s.openFrom != 0 {
		if s.openFrom > total {
			return nil, fmt.Errorf("page %d does not exist (page_count=%d)", s.openFrom, total)
		}
		for page := s.openFrom; page <= total; page++ {
			seen[page] = struct{}{}
		}
	}
	pages := make([]int, 0, len(seen))
	for page := range seen {
		pages = append(pages, page)
	}
	sort.Ints(pages)
	return pages, nil
}

// ClosedPages 返回闭区间选择的防御性副本。第二个返回值仅在选择包含开区间（如
// 3-）时为 false；这类范围必须等待已知的作品实际页数后再调用 Resolve。
func (s *PageSelection) ClosedPages() ([]int, bool) {
	if s == nil {
		return nil, true
	}
	if s.openFrom != 0 {
		return nil, false
	}
	return append([]int(nil), s.pages...), true
}

// DownloadProgress 同时报告单个资源和整个批次的已传输字节。TotalBytesKnown 为
// true 仅表示全部资源在下载前都由安全 HEAD 确定了大小；未知总量时仍持续报告
// ResourceBytesTransferred 与 TotalBytesTransferred。SourceIndex 对应 srcs 的输入序号。
type DownloadProgress struct {
	SourceIndex     int    `json:"source_index"`
	Page            int    `json:"page,omitempty"`
	DestinationPath string `json:"destination_path"`
	IllustID        int64  `json:"illust_id,omitempty"`
	Title           string `json:"title,omitempty"`
	Author          string `json:"author,omitempty"`

	ResourceBytesTransferred int64 `json:"resource_bytes_transferred"`
	ResourceTotalBytes       int64 `json:"resource_total_bytes,omitempty"`
	ResourceTotalKnown       bool  `json:"resource_total_known"`
	TotalBytesTransferred    int64 `json:"total_bytes_transferred"`
	TotalBytes               int64 `json:"total_bytes,omitempty"`
	TotalBytesKnown          bool  `json:"total_bytes_known"`
	CompletedResources       int   `json:"completed_resources"`
	TotalResources           int   `json:"total_resources"`
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

// ValidateUgoiraFormat 校验 ugoira 输出容器。空值是兼容默认 GIF，调用方不必显式填写。
func ValidateUgoiraFormat(format UgoiraFormat) error {
	return ValidateUgoiraMode(UgoiraMode(format))
}

// ValidateUgoiraMode 校验 ugoira 最终产物模式。
func ValidateUgoiraMode(mode UgoiraMode) error {
	switch mode {
	case "", UgoiraModeGIF, UgoiraModeAPNG, UgoiraModeZIP, UgoiraModeFrames:
		return nil
	default:
		return fmt.Errorf("ugoira mode must be one of gif, apng, zip, frames")
	}
}
