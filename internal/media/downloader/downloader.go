// Package downloader 把用户给出的 Pixiv 来源展开成作品，再把每件作品的图片或
// 动图写入下载目录。来源解析、批量执行、单作品下载和资源命名分别在
// sources.go、batch.go、artwork.go、resource.go。
package downloader

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/logging"
	"github.com/FlanChanXwO/pixiv-cli/internal/pagination"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// DownloadClient 是下载实现从 public SDK operation snapshot 使用的最小能力集。
// ResourceRef 由共享 sdk core 的 ParseResourceRef 解析；下载器只消费 opaque ref。
type DownloadClient interface {
	Artwork(context.Context, pixiv.ArtworkRequest) (pixiv.Artwork, error)
	UgoiraMetadata(context.Context, pixiv.UgoiraMetadataRequest) (pixiv.UgoiraMetadata, error)
	SaveResource(context.Context, sdk.ResourceRef, sdk.SaveOptions) (sdk.SavedResource, error)
}

// BatchDownloader 下载一批已经解析出 ID 的作品。
type BatchDownloader interface {
	Download(context.Context, DownloadRequest) (DownloadReport, error)
}

// DownloadService 编排来源展开、批量下载和部分成功报告。
type DownloadService struct {
	// NewBatchDownloader 构造一次请求使用的批量下载器；composition root 注入文件系统实现。
	NewBatchDownloader func(client DownloadClient, downloadPath, filenameTemplate string) (BatchDownloader, error)
}

// DownloadSources 是 CLI/MCP 的入口：作品 PID、作品 URL、受资源策略允许的直链
// 都原样交给 public SDK 解析。用户主页 URL 是唯一的本地展开规则，因为它代表
// 一组作品而非单个 SDK 下载来源。
func (s DownloadService) DownloadSources(ctx context.Context, client DownloadTargetClient, sources []string, request DownloadRequest) (report DownloadReport, returnErr error) {
	startedAt := time.Now()
	report = DownloadReport{Items: []DownloadedArtwork{}, Failures: []DownloadFailure{}}
	defer func() {
		if returnErr != nil {
			logging.From(ctx).WarnContext(ctx, "Download sources failed",
				slog.String(logging.AttrReason, "command failed"),
				slog.Int(logging.AttrCount, len(report.Items)),
				slog.Duration(logging.AttrDuration, time.Since(startedAt)))
			return
		}
		logging.From(ctx).DebugContext(ctx, "Download completed",
			slog.Int(logging.AttrCount, len(report.Items)),
			slog.Duration(logging.AttrDuration, time.Since(startedAt)))
	}()
	if len(sources) == 0 {
		return report, errors.New("at least one download source is required")
	}

	plan, err := classifySources(ctx, client, sources, &report)
	if err != nil {
		return report, err
	}
	if err := s.downloadArtworkIDs(ctx, client, plan.artworkIDs, request, &report); err != nil {
		return report, err
	}
	if err := downloadResourceRefs(ctx, client, plan.resourceRefs, request.DownloadPath, &report); err != nil {
		return report, err
	}
	if err := downloadDirectURLs(ctx, client, plan.directURLs, request.DownloadPath, &report); err != nil {
		return report, err
	}
	return report, nil
}

// sourcePlan 是来源分类的结果：已展开的作品 ID 和仍需下载的直链/资源引用。
type sourcePlan struct {
	artworkIDs   []int64
	resourceRefs []sdk.ResourceRef
	directURLs   []string
}

// classifySources 按来源类型分流：作品与用户页展开为 ID，直链与 opaque ref
// 单独保留。无法识别的来源记为单项失败，不中断其余来源。
func classifySources(ctx context.Context, client DownloadTargetClient, sources []string, report *DownloadReport) (sourcePlan, error) {
	var plan sourcePlan
	seen := make(map[int64]struct{})
	appendArtwork := func(id int64) {
		if id <= 0 {
			return
		}
		if _, duplicate := seen[id]; duplicate {
			return
		}
		seen[id] = struct{}{}
		plan.artworkIDs = append(plan.artworkIDs, id)
	}
	for _, source := range sources {
		if err := ctx.Err(); err != nil {
			return plan, err
		}
		reference, err := pixiv.ParseURL(source)
		if err == nil {
			switch reference.Kind {
			case pixiv.ReferenceKindArtwork:
				appendArtwork(reference.ID)
				continue
			case pixiv.ReferenceKindUser:
				if err := collectUserArtworkJobs(ctx, client, reference, appendArtwork, report); err != nil {
					return plan, err
				}
				continue
			case pixiv.ReferenceKindUserBookmarks:
				if err := collectUserBookmarkJobs(ctx, client, reference, appendArtwork); err != nil {
					return plan, err
				}
				continue
			default:
				return plan, errors.New("download source is invalid")
			}
		}
		if id, parseErr := strconv.ParseInt(strings.TrimSpace(source), 10, 64); parseErr == nil && id > 0 {
			appendArtwork(id)
			continue
		}
		if isDirectResourceURL(source) {
			plan.directURLs = append(plan.directURLs, strings.TrimSpace(source))
			continue
		}
		ref, refErr := sdk.ParseResourceRef(source)
		if refErr != nil {
			report.Failures = append(report.Failures, DownloadFailure{
				URL: redactedDownloadSource, Message: "download source is invalid", Cause: refErr,
			})
			continue
		}
		plan.resourceRefs = append(plan.resourceRefs, ref)
	}
	return plan, nil
}

// downloadArtworkIDs 把展开后的作品交给批量下载器，并把部分结果并入报告。
func (s DownloadService) downloadArtworkIDs(ctx context.Context, client DownloadClient, artworkIDs []int64, request DownloadRequest, report *DownloadReport) error {
	if len(artworkIDs) == 0 {
		return nil
	}
	downloader, err := s.NewBatchDownloader(client, request.DownloadPath, request.FilenameTemplate)
	if err != nil {
		return err
	}
	request.IllustIDs = artworkIDs
	batch, err := downloader.Download(ctx, request)
	report.Items = append(report.Items, batch.Items...)
	report.Failures = append(report.Failures, batch.Failures...)
	report.Warnings = append(report.Warnings, batch.Warnings...)
	report.Committed = report.Committed || len(batch.Items) > 0
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		// batch 中的单项业务失败已经由 Failures 表达；返回值仅保留
		// 无法继续当前 operation 的错误，不再把它伪装成单个失败项。
		return err
	}
	return nil
}

// downloadResourceRefs 直接保存 opaque 资源引用；失败只记录该项。
func downloadResourceRefs(ctx context.Context, client DownloadClient, refs []sdk.ResourceRef, downloadPath string, report *DownloadReport) error {
	for _, ref := range refs {
		if err := ctx.Err(); err != nil {
			return err
		}
		path, err := directResourcePath(downloadPath, ref)
		if err != nil {
			report.Failures = append(report.Failures, DownloadFailure{URL: ref.String(), Message: err.Error(), Cause: err})
			continue
		}
		if _, err := client.SaveResource(ctx, ref, sdk.SaveOptions{Path: path}); err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			report.Failures = append(report.Failures, DownloadFailure{URL: ref.String(), Message: err.Error(), Cause: err})
			continue
		}
		report.Items = append(report.Items, DownloadedArtwork{
			Type:  downloadedResourceType,
			Files: []DownloadedFile{{Path: path, Page: 1}},
		})
		report.Committed = true
	}
	return nil
}

// downloadDirectURLs 保存受资源策略允许的直链；原始 URL 与 query 不进入报告。
func downloadDirectURLs(ctx context.Context, client DownloadTargetClient, rawURLs []string, downloadPath string, report *DownloadReport) error {
	directClient, supportsDirectURLs := client.(DirectResourceClient)
	for _, rawURL := range rawURLs {
		if err := ctx.Err(); err != nil {
			return err
		}
		path, err := directResourcePathFromURL(downloadPath, rawURL)
		if err != nil {
			report.Failures = append(report.Failures, DownloadFailure{
				URL: redactedDownloadSource, Type: downloadedResourceType, Message: err.Error(), Cause: err,
			})
			continue
		}
		if !supportsDirectURLs {
			err := errors.New("download client does not support direct resource URLs")
			report.Failures = append(report.Failures, DownloadFailure{
				URL: redactedDownloadSource, Type: downloadedResourceType, Message: err.Error(), Cause: err,
			})
			continue
		}
		if _, err := directClient.SaveResourceURL(ctx, rawURL, sdk.SaveOptions{Path: path}); err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			safeErr := redactDirectResourceError(rawURL, err)
			report.Failures = append(report.Failures, DownloadFailure{
				URL: redactedDownloadSource, Type: downloadedResourceType, Message: safeErr.Error(), Cause: safeErr,
			})
			continue
		}
		report.Items = append(report.Items, DownloadedArtwork{
			Type:  downloadedResourceType,
			Files: []DownloadedFile{{Path: path, Page: 1}},
		})
		report.Committed = true
	}
	return nil
}

func collectUserArtworkJobs(ctx context.Context, client DownloadTargetClient, user pixiv.Reference, appendArtwork func(int64), report *DownloadReport) error {
	for _, kind := range []pixiv.ArtworkKind{pixiv.ArtworkKindIllustration, pixiv.ArtworkKindManga, pixiv.ArtworkKindUgoira} {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err := pagination.TraversePages(ctx, pagination.PagePlan{}, func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
			result, err := client.UserArtworks(ctx, pixiv.UserArtworksRequest{UserID: user.ID, Kind: kind, Cursor: cursor})
			if err != nil {
				return nil, sdk.Cursor{}, err
			}
			return result.Items, result.Next, nil
		}, func(items []pixiv.Artwork) error {
			for _, artwork := range items {
				appendArtwork(artwork.ID)
			}
			return nil
		})
		if err == nil {
			continue
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		report.Failures = append(report.Failures, DownloadFailure{
			URL: referenceURL(user), Type: string(kind), Message: err.Error(), Cause: err,
		})
	}
	return nil
}

func collectUserBookmarkJobs(ctx context.Context, client DownloadTargetClient, user pixiv.Reference, appendArtwork func(int64)) error {
	_, err := pagination.TraversePages(ctx, pagination.PagePlan{}, func(ctx context.Context, cursor sdk.Cursor) ([]pixiv.Artwork, sdk.Cursor, error) {
		result, err := client.UserArtworkBookmarks(ctx, pixiv.UserArtworkBookmarksRequest{UserID: user.ID, Restrict: pixiv.RestrictPublic, Cursor: cursor})
		if err != nil {
			return nil, sdk.Cursor{}, err
		}
		return result.Items, result.Next, nil
	}, func(items []pixiv.Artwork) error {
		for _, artwork := range items {
			appendArtwork(artwork.ID)
		}
		return nil
	})
	return err
}

func referenceURL(reference pixiv.Reference) string {
	raw, err := reference.CanonicalURL()
	if err != nil {
		return ""
	}
	return raw
}
