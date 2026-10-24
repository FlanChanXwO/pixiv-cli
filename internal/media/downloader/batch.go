// 批量下载：按当前运行时并行度处理作品，并按输入顺序汇总产物、失败与提示。

package downloader

import (
	"context"
	"errors"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/FlanChanXwO/pixiv-cli/internal/media/filename"
)

// ArtworkDownloader 把一批作品写入同一个下载目录，并报告每件作品的结果。
type ArtworkDownloader struct {
	client           DownloadClient
	downloadPath     string
	filenameTemplate string
}

// NewArtworkDownloader 构造批量下载；ugoira 编码器使用包内默认实现。
func NewArtworkDownloader(client DownloadClient, downloadPath, filenameTemplate string) *ArtworkDownloader {
	return &ArtworkDownloader{client: client, downloadPath: downloadPath, filenameTemplate: filenameTemplate}
}

// Download 下载 request 中的作品，并按输入顺序返回成功项、失败项和提示。
func (b *ArtworkDownloader) Download(ctx context.Context, request DownloadRequest) (DownloadReport, error) {
	unique := deduplicatePositive(request.IllustIDs)
	quality := request.Quality
	if quality == "" {
		quality = DownloadQualityOriginal
	}
	if err := ValidateDownloadQuality(quality); err != nil {
		return DownloadReport{}, err
	}
	ugoiraFormat := request.UgoiraFormat
	if ugoiraFormat == "" {
		ugoiraFormat = UgoiraFormatGIF
	}
	if err := ValidateUgoiraFormat(ugoiraFormat); err != nil {
		return DownloadReport{}, err
	}
	directoryTemplate := strings.TrimSpace(request.DirectoryTemplate)
	if err := filename.ValidateDirectoryTemplate(directoryTemplate); err != nil {
		return DownloadReport{}, err
	}
	report := DownloadReport{
		Items:    []DownloadedArtwork{},
		Failures: []DownloadFailure{},
		Warnings: []DownloadWarning{},
	}
	results := make([]struct {
		artwork  DownloadedArtwork
		warnings []DownloadWarning
		err      error
		done     bool
	}, len(unique))
	parallelErr := forEach(ctx, len(unique), func(ctx context.Context, index int) {
		results[index].artwork, results[index].warnings, results[index].err = b.downloadArtwork(ctx, unique[index], request.Pages, quality, ugoiraFormat, directoryTemplate)
		results[index].done = true
	})
	var workerContextErr error
	for index, result := range results {
		if !result.done {
			continue
		}
		report.Warnings = append(report.Warnings, result.warnings...)
		if result.err == nil {
			report.Items = append(report.Items, result.artwork)
			continue
		}
		// 作品内已有文件时，保留 partial item；失败项仍单独进入 Failures，
		// 让批量报告与磁盘上已经发布的文件保持一致。
		if len(result.artwork.Files) > 0 {
			report.Items = append(report.Items, result.artwork)
		}
		if errors.Is(result.err, context.Canceled) || errors.Is(result.err, context.DeadlineExceeded) {
			if workerContextErr == nil {
				workerContextErr = result.err
			}
			continue
		}
		report.Failures = append(report.Failures, DownloadFailure{
			IllustID: unique[index],
			Message:  result.err.Error(),
			Cause:    result.err,
		})
	}
	if parallelErr != nil {
		return report, parallelErr
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return report, ctxErr
	}
	if workerContextErr != nil {
		return report, workerContextErr
	}
	return report, nil
}

func deduplicatePositive(values []int64) []int64 {
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

// forEach 在当前运行时可用并行度内处理每个索引。work 负责把各索引的业务失败写入
// 自己的结果槽，避免一个作品失败取消同一批的其他作品；context 取消仍立即停止派发。
func forEach(ctx context.Context, tasks int, work func(context.Context, int)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	workers := workerCount(tasks)
	if workers == 0 {
		return nil
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
					work(ctx, index)
				}
			}
		}()
	}
	for index := range tasks {
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

// workerCount 把系统可用并发收敛到本批任务数，既不创建空 worker，也不以固定数字
// 限制用户的下载任务。
func workerCount(tasks int) int {
	if tasks <= 0 {
		return 0
	}
	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}
	if tasks < workers {
		return tasks
	}
	return workers
}
