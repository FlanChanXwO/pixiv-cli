package pixiv

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/FlanChanXwO/pixiv-cli/internal/ugoira"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/filename"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/text"
	uriutil "github.com/FlanChanXwO/pixiv-cli/internal/utils/uri"
)

// DownloadSourceKind 标识下载输入经自动解析后的来源类别。
type DownloadSourceKind string

const (
	DownloadSourceArtwork  DownloadSourceKind = "artwork"
	DownloadSourceResource DownloadSourceKind = "resource"
)

// DownloadedFile 是一个已成功写入本地的文件。
type DownloadedFile struct {
	Path       string                     `json:"path"`
	Page       int                        `json:"page,omitempty"`
	CacheState ResourceDownloadCacheState `json:"cache_state"`
}

// DownloadResult 是单个 src 成功完成后的结果。直链资源不包含作品元数据。
type DownloadResult struct {
	SourceKind DownloadSourceKind `json:"source_kind"`
	IllustID   int64              `json:"illust_id,omitempty"`
	Title      string             `json:"title,omitempty"`
	Author     string             `json:"author,omitempty"`
	Type       string             `json:"type,omitempty"`
	Files      []DownloadedFile   `json:"files"`
}

// DownloadItemResult 保存批量下载中一个输入的完成状态。未启动项只会在调用方取消时出现。
type DownloadItemResult struct {
	Attempted bool `json:"attempted"`
	// Committed 表示该输入已经发布至少一个用户可见的本地文件。即使随后另一
	// 资源失败，调用方也不得切换账号重放这个输入。
	Committed bool            `json:"committed"`
	Result    *DownloadResult `json:"result,omitempty"`
	Err       error           `json:"-"`
}

// DownloadAllResult 保持 srcs 的输入顺序；调用方可筛选 Err 或 Attempted 以定向重试。
type DownloadAllResult struct {
	Items []DownloadItemResult `json:"items"`
}

type downloadSource struct {
	kind DownloadSourceKind
	id   int64
	ref  ResourceRef
}

type preparedResource struct {
	ref         ResourceRef
	destination string
	fileIndex   int
}

type preparedDownload struct {
	result    DownloadResult
	resources []preparedResource
	animation *preparedUgoira
}

type downloadTask struct {
	itemIndex int
	resource  preparedResource
	result    DownloadResult
}

// preparedUgoira 将 ZIP 缓存资源与最终用户可见动图分开：ZIP 只留在 .pixiv-cache，
// GIF 才是 DownloadResult 中的产物。
type preparedUgoira struct {
	zipPath    string
	frames     []ugoira.Frame
	workDir    string
	outputPath string
	format     UgoiraFormat
	fileIndex  int
}

// Download 以公开默认值下载一个 PID、作品 URL 或资源直链。
func (c *Client) Download(ctx context.Context, src string) (DownloadResult, error) {
	return c.DownloadWith(ctx, src, DownloadOptions{})
}

// DownloadAll 以公开默认值下载多个 PID、作品 URL 或资源直链。
func (c *Client) DownloadAll(ctx context.Context, srcs []string) (DownloadAllResult, error) {
	return c.DownloadAllWith(ctx, srcs, DownloadOptions{})
}

// DownloadWith 以显式路径、模板、页码、质量和并发设置下载一个来源。
func (c *Client) DownloadWith(ctx context.Context, src string, options DownloadOptions) (DownloadResult, error) {
	result, err := c.DownloadAllWith(ctx, []string{src}, options)
	if err != nil {
		return DownloadResult{}, err
	}
	item := result.Items[0]
	if item.Err != nil {
		return DownloadResult{}, item.Err
	}
	if item.Result == nil {
		return DownloadResult{}, errors.New("download did not produce a result")
	}
	return *item.Result, nil
}

// DownloadAllWith 批量下载来源。单项失败保留在结果中；只有参数整体非法或 context
// 取消时才返回顶层错误。
func (c *Client) DownloadAllWith(ctx context.Context, srcs []string, options DownloadOptions) (out DownloadAllResult, err error) {
	if len(srcs) == 0 {
		return DownloadAllResult{}, invalidResourceError(OperationDownload, "at least one download source is required")
	}
	if scoped, snapshotErr := c.operationClient(ctx, OperationDownload); snapshotErr != nil {
		return DownloadAllResult{}, snapshotErr
	} else if scoped != c {
		return scoped.DownloadAllWith(ctx, srcs, options)
	}
	options, workers, err := normalizeDownloadOptions(options)
	if err != nil {
		return DownloadAllResult{}, err
	}
	if err := os.MkdirAll(options.DownloadPath, 0o755); err != nil {
		return DownloadAllResult{}, invalidResourceError(OperationDownload, "cannot create download directory")
	}
	base, err := filepath.Abs(options.DownloadPath)
	if err != nil {
		return DownloadAllResult{}, invalidResourceError(OperationDownload, "download path is invalid")
	}
	options.DownloadPath = base

	out.Items = make([]DownloadItemResult, len(srcs))
	prepared := make([]*preparedDownload, len(srcs))
	destinations := make(map[string]int)
	for index, src := range srcs {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		plan, planErr := c.prepareDownload(ctx, src, options)
		if planErr != nil {
			out.Items[index].Err = planErr
			continue
		}
		collision := false
		for _, resource := range plan.resources {
			key := filepath.Clean(resource.destination)
			if existing, ok := destinations[key]; ok {
				out.Items[index].Err = fmt.Errorf("download destination conflicts with source at index %d", existing)
				collision = true
				break
			}
			destinations[key] = index
		}
		if collision {
			continue
		}
		prepared[index] = plan
	}

	tasks := make([]downloadTask, 0)
	for itemIndex, plan := range prepared {
		if plan == nil {
			continue
		}
		for _, resource := range plan.resources {
			tasks = append(tasks, downloadTask{itemIndex: itemIndex, resource: resource, result: plan.result})
		}
	}
	var tracker *downloadProgressTracker
	if options.Progress != nil {
		progressResources, totalKnown, probeErr := c.prepareDownloadProgress(ctx, tasks)
		if probeErr != nil {
			return out, probeErr
		}
		tracker = newDownloadProgressTracker(options.Progress, progressResources, totalKnown)
	}
	resourceErrors := make([]error, len(tasks))
	resourceResults := make([]ResourceDownloadResult, len(tasks))
	attempted := make([]atomic.Bool, len(srcs))
	if err := runDownloadTasks(ctx, workers, len(tasks), func(index int) {
		item := tasks[index]
		attempted[item.itemIndex].Store(true)
		if tracker != nil {
			tracker.start(index)
		}
		resourceResults[index], resourceErrors[index] = c.downloadResourceWithProgress(ctx, item.resource.ref, item.resource.destination, func(bytes int64) {
			if tracker != nil {
				tracker.add(index, bytes)
			}
		})
		if resourceErrors[index] == nil && tracker != nil {
			tracker.complete(index)
		}
	}); err != nil {
		for index := range out.Items {
			out.Items[index].Attempted = attempted[index].Load()
		}
		return out, err
	}
	for index := range out.Items {
		out.Items[index].Attempted = attempted[index].Load()
	}
	for taskIndex, task := range tasks {
		if resourceErrors[taskIndex] != nil {
			if out.Items[task.itemIndex].Err == nil {
				out.Items[task.itemIndex].Err = resourceErrors[taskIndex]
			}
			continue
		}
		plan := prepared[task.itemIndex]
		plan.result.Files[task.resource.fileIndex].CacheState = resourceResults[taskIndex].CacheState
		if plan.animation == nil {
			out.Items[task.itemIndex].Committed = true
		}
	}
	for index, plan := range prepared {
		if plan == nil || out.Items[index].Err != nil {
			continue
		}
		if plan.animation != nil {
			if err := ugoira.NewRustEncoder().Encode(ctx, ugoira.Input{
				ZipPath:    plan.animation.zipPath,
				Frames:     plan.animation.frames,
				WorkDir:    plan.animation.workDir,
				OutputPath: plan.animation.outputPath,
				Format:     ugoira.Format(plan.animation.format),
			}); err != nil {
				out.Items[index].Err = err
				continue
			}
			out.Items[index].Committed = true
		}
		result := plan.result
		out.Items[index].Result = &result
	}
	return out, nil
}

// prepareDownloadProgress 逐个安全探测资源大小。任何一次 HEAD 无法确定大小时，
// 批次总量保持 unknown；实际下载仍会继续，单资源传输事件不受影响。
func (c *Client) prepareDownloadProgress(ctx context.Context, tasks []downloadTask) ([]downloadProgressResource, bool, error) {
	resources := make([]downloadProgressResource, len(tasks))
	totalKnown := true
	for index, task := range tasks {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		file := task.result.Files[task.resource.fileIndex]
		total, known, err := c.probeResource(ctx, task.resource.ref)
		if err != nil {
			known = false
		}
		initial := c.verifiedPartialBytes(task.resource.ref, task.resource.destination)
		if known && initial > total {
			// validator 相同却出现超出 HEAD 总长的缓存不应制造错误进度条；下载缓存
			// 路径随后会按真实响应验证或失败，不在预检阶段隐瞒该状态。
			known = false
		}
		if !known {
			totalKnown = false
		}
		resources[index] = downloadProgressResource{base: DownloadProgress{
			SourceIndex: task.itemIndex, Page: file.Page, DestinationPath: task.resource.destination,
			IllustID: task.result.IllustID, Title: task.result.Title, Author: task.result.Author,
			ResourceTotalBytes: total, ResourceTotalKnown: known,
		}, initial: initial}
	}
	return resources, totalKnown, nil
}

func normalizeDownloadOptions(options DownloadOptions) (DownloadOptions, int, error) {
	options.DownloadPath = strings.TrimSpace(options.DownloadPath)
	if options.DownloadPath == "" {
		options.DownloadPath = DefaultDownloadPath
	}
	options.FilenameTemplate = strings.TrimSpace(options.FilenameTemplate)
	if options.FilenameTemplate == "" {
		options.FilenameTemplate = DefaultFilenameTemplate
	}
	if err := filename.ValidateTemplate(options.FilenameTemplate); err != nil {
		return DownloadOptions{}, 0, invalidResourceError(OperationDownload, err.Error())
	}
	if options.Quality == "" {
		options.Quality = DownloadQualityOriginal
	}
	if err := ValidateDownloadQuality(options.Quality); err != nil {
		return DownloadOptions{}, 0, err
	}
	if options.UgoiraFormat == "" {
		options.UgoiraFormat = UgoiraFormatGIF
	}
	if err := ValidateUgoiraFormat(options.UgoiraFormat); err != nil {
		return DownloadOptions{}, 0, invalidResourceError(OperationDownload, err.Error())
	}
	if options.Concurrency < 0 {
		return DownloadOptions{}, 0, invalidResourceError(OperationDownload, "download concurrency must not be negative")
	}
	workers := options.Concurrency
	if workers == 0 {
		workers = runtime.GOMAXPROCS(0) * 2
		if workers < 1 {
			workers = 1
		}
	}
	return options, workers, nil
}

func (c *Client) prepareDownload(ctx context.Context, src string, options DownloadOptions) (*preparedDownload, error) {
	source, err := c.parseDownloadSource(src)
	if err != nil {
		return nil, err
	}
	switch source.kind {
	case DownloadSourceResource:
		if err := validateDirectDownloadOptions(options); err != nil {
			return nil, err
		}
		name := filename.Sanitize(filepath.Base(uriutil.PathFromURL(source.ref.URL)))
		if name == "" || name == "." || name == string(filepath.Separator) {
			return nil, invalidResourceError(OperationDownload, "resource URL has no usable filename")
		}
		path := filepath.Join(options.DownloadPath, name)
		return &preparedDownload{
			result:    DownloadResult{SourceKind: DownloadSourceResource, Files: []DownloadedFile{{Path: path}}},
			resources: []preparedResource{{ref: source.ref, destination: path, fileIndex: 0}},
		}, nil
	case DownloadSourceArtwork:
		return c.prepareArtworkDownload(ctx, source.id, options)
	default:
		return nil, invalidResourceError(OperationDownload, "download source is invalid")
	}
}

func (c *Client) parseDownloadSource(src string) (downloadSource, error) {
	if id, err := ParseArtworkReference(src); err == nil {
		return downloadSource{kind: DownloadSourceArtwork, id: id}, nil
	}
	ref, err := c.ParseResourceRef(src)
	if err != nil {
		return downloadSource{}, invalidResourceError(OperationDownload, "download source is invalid")
	}
	return downloadSource{kind: DownloadSourceResource, ref: ref}, nil
}

func (c *Client) prepareArtworkDownload(ctx context.Context, id int64, options DownloadOptions) (*preparedDownload, error) {
	detail, err := c.IllustDetail(ctx, id)
	if err != nil {
		return nil, err
	}
	illust := detail.Illust
	if illust.Type == string(IllustTypeUgoira) {
		return c.prepareUgoiraDownload(ctx, illust, options)
	}
	pages, err := selectDownloadPages(illust, options.Pages)
	if err != nil {
		return nil, err
	}
	base, err := filepath.Abs(options.DownloadPath)
	if err != nil {
		return nil, invalidResourceError(OperationDownload, "download path is invalid")
	}
	plan := &preparedDownload{result: DownloadResult{
		SourceKind: DownloadSourceArtwork,
		IllustID:   illust.ID,
		Title:      illust.Title,
		Author:     illust.User.Name,
		Type:       illust.Type,
		Files:      make([]DownloadedFile, len(pages)),
	}}
	for index, page := range pages {
		rawURL, err := selectDownloadImageURL(page.urls, illust.MetaSinglePage.OriginalImageURL, options.Quality)
		if err != nil {
			return nil, err
		}
		ref, err := c.ParseResourceRef(rawURL)
		if err != nil {
			return nil, err
		}
		name := filename.Generate(downloadFilenameData(illust), page.page1-1, options.FilenameTemplate) + downloadExtension(rawURL)
		path := filepath.Join(base, name)
		plan.result.Files[index] = DownloadedFile{Path: path, Page: page.page1}
		plan.resources = append(plan.resources, preparedResource{ref: ref, destination: path, fileIndex: index})
	}
	return plan, nil
}

type downloadPage struct {
	page1 int
	urls  ImageURLs
}

func selectDownloadPages(illust Illust, pages []int) ([]downloadPage, error) {
	total := illust.PageCount
	if total <= 0 {
		total = 1
	}
	if total == 1 && len(illust.MetaPages) == 0 {
		if len(pages) > 0 {
			for _, page := range pages {
				if page != 1 {
					return nil, fmt.Errorf("page %d does not exist (page_count=1)", page)
				}
			}
		}
		return []downloadPage{{page1: 1, urls: illust.ImageURLs}}, nil
	}
	if len(illust.MetaPages) < total {
		return nil, errors.New("illustration pages are unavailable")
	}
	want := make(map[int]struct{}, total)
	if len(pages) == 0 {
		for page := 1; page <= total; page++ {
			want[page] = struct{}{}
		}
	} else {
		for _, page := range pages {
			if page < 1 || page > total {
				return nil, fmt.Errorf("page %d does not exist (page_count=%d)", page, total)
			}
			want[page] = struct{}{}
		}
	}
	result := make([]downloadPage, 0, len(want))
	for index, page := range illust.MetaPages {
		page1 := index + 1
		if _, ok := want[page1]; ok {
			result = append(result, downloadPage{page1: page1, urls: page.ImageURLs})
		}
	}
	return result, nil
}

func validateDirectDownloadOptions(options DownloadOptions) error {
	if len(options.Pages) > 0 {
		return invalidResourceError(OperationDownload, "page selection is only supported for Pixiv artworks")
	}
	if options.Quality != DownloadQualityOriginal {
		return invalidResourceError(OperationDownload, "image quality is only supported for Pixiv artworks")
	}
	if options.FilenameTemplate != DefaultFilenameTemplate {
		return invalidResourceError(OperationDownload, "filename template is only supported for Pixiv artworks")
	}
	return nil
}

func (c *Client) prepareUgoiraDownload(ctx context.Context, illust Illust, options DownloadOptions) (*preparedDownload, error) {
	if len(options.Pages) > 0 {
		return nil, invalidResourceError(OperationDownload, "page selection is unsupported for ugoira")
	}
	if options.Quality != DownloadQualityOriginal {
		return nil, invalidResourceError(OperationDownload, "image quality is unsupported for ugoira")
	}
	metadata, err := c.UgoiraMetadata(ctx, illust.ID)
	if err != nil {
		return nil, err
	}
	zipURL := metadata.UgoiraMetadata.DownloadURL
	if zipURL == "" {
		return nil, newError(CodeMalformedUpstreamResponse, OperationDownload, BackendAppAPI, false, 0, illust.ID, errors.New("ugoira metadata has no download URL"))
	}
	ref, err := c.ParseResourceRef(zipURL)
	if err != nil {
		return nil, err
	}
	base := options.DownloadPath
	name := filename.Generate(downloadFilenameData(illust), 0, options.FilenameTemplate)
	outputPath := filepath.Join(base, name+"."+string(options.UgoiraFormat))
	zipPath := filepath.Join(c.resourceCacheDirectory(base), "ugoira", fmt.Sprintf("%d.zip", illust.ID))
	if err := os.MkdirAll(filepath.Dir(zipPath), 0o700); err != nil {
		return nil, invalidResourceError(OperationDownload, "cannot create ugoira download cache")
	}
	frames := make([]ugoira.Frame, len(metadata.UgoiraMetadata.Frames))
	for index, frame := range metadata.UgoiraMetadata.Frames {
		frames[index] = ugoira.Frame{File: frame.File, Delay: frame.Delay}
	}
	return &preparedDownload{
		result: DownloadResult{
			SourceKind: DownloadSourceArtwork,
			IllustID:   illust.ID,
			Title:      illust.Title,
			Author:     illust.User.Name,
			Type:       illust.Type,
			Files:      []DownloadedFile{{Path: outputPath, Page: 1}},
		},
		resources: []preparedResource{{ref: ref, destination: zipPath, fileIndex: 0}},
		animation: &preparedUgoira{
			zipPath: zipPath, frames: frames, workDir: base, outputPath: outputPath, format: options.UgoiraFormat, fileIndex: 0,
		},
	}, nil
}

func selectDownloadImageURL(urls ImageURLs, singleOriginal string, quality DownloadQuality) (string, error) {
	switch quality {
	case DownloadQualityOriginal:
		if raw := text.FirstNonEmpty(singleOriginal, urls.Original, urls.Large); raw != "" {
			return raw, nil
		}
	case DownloadQualityRegular:
		if raw := text.FirstNonEmpty(urls.Large, urls.Medium, urls.Original, singleOriginal); raw != "" {
			return raw, nil
		}
	case DownloadQualitySmall:
		if raw := text.FirstNonEmpty(urls.Medium, urls.Large, urls.Original, singleOriginal); raw != "" {
			return raw, nil
		}
	case DownloadQualityThumb, DownloadQualityMini:
		if raw := text.FirstNonEmpty(urls.SquareMedium, urls.Medium, urls.Large, urls.Original, singleOriginal); raw != "" {
			return raw, nil
		}
	}
	return "", errors.New("selected image quality has no resource URL")
}

func downloadFilenameData(illust Illust) filename.FilenameData {
	return filename.FilenameData{ID: illust.ID, Author: illust.User.Name, Title: illust.Title, PageCount: illust.PageCount}
}

func downloadExtension(rawURL string) string {
	extension := filename.Sanitize(filepath.Ext(uriutil.PathFromURL(rawURL)))
	extension = strings.Map(func(character rune) rune {
		if character < 0x20 || character == 0x7f {
			return '_'
		}
		return character
	}, extension)
	return strings.TrimRight(extension, ". ")
}

func runDownloadTasks(ctx context.Context, workers, count int, work func(int)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if count == 0 {
		return nil
	}
	if workers > count {
		workers = count
	}
	jobs := make(chan int)
	var group sync.WaitGroup
	group.Add(workers)
	for range workers {
		go func() {
			defer group.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case index, ok := <-jobs:
					if !ok {
						return
					}
					work(index)
				}
			}
		}()
	}
	for index := range count {
		select {
		case <-ctx.Done():
			close(jobs)
			group.Wait()
			return ctx.Err()
		case jobs <- index:
		}
	}
	close(jobs)
	group.Wait()
	return ctx.Err()
}
