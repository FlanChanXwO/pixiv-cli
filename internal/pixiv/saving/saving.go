// Package saving 展开 CLI 与本地 MCP 的下载来源，并保留逐作品报告。
//
// [Save] 在调用方选定的账号内展开来源；文件保存与动图编码交给 sdk/download。
// [Request] 接收调用方解析的 1-based 页码与路径、格式选项。
package saving

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"slices"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/media/filename"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/download"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// Save 对来源分类、去重并按作品 ID 排序下载；失败保留为报告，取消返回已发布的部分结果。
func Save(ctx context.Context, client *pixiv.Client, sources []string, request Request) (DownloadReport, error) {
	report := DownloadReport{Items: []DownloadedArtwork{}, Failures: []DownloadFailure{}}
	if len(sources) == 0 {
		return report, errors.New("at least one download source is required")
	}
	ids := map[int64]bool{}
	appendPage := func(page sdk.Page[*pixiv.Artwork], err error) error {
		for err == nil {
			for _, art := range page.Items {
				ids[art.ID] = true
			}
			if page.Cursor.IsZero() {
				break
			}
			page, err = page.Next(ctx)
		}
		return err
	}
	var refs []resourceReference
	var urls []string
	// 1. 展开来源；作者的某一作品类型失败时仍处理其余类型。
	for _, source := range sources {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		ref, err := client.ParseURL(source)
		if err == nil {
			switch ref := ref.(type) {
			case *pixiv.ArtworkRef:
				ids[ref.ID] = true
			case *pixiv.UserBookmarksRef:
				if err := appendPage(ref.ArtworkBookmarks(ctx, pixiv.ArtworkBookmarksOptions{Restrict: pixiv.RestrictPublic})); err != nil {
					return report, err
				}
			case *pixiv.UserRef:
				for _, kind := range []pixiv.ContentType{pixiv.ContentTypeIllust, pixiv.ContentTypeManga, pixiv.ContentTypeUgoira} {
					if err := appendPage(ref.Artworks(ctx, pixiv.UserArtworksOptions{Kind: kind})); err != nil {
						if ctx.Err() != nil {
							return report, ctx.Err()
						}
						url, _ := ref.CanonicalURL()
						report.Failures = append(report.Failures, DownloadFailure{URL: url, Type: string(kind), Message: downloadError("UserArtworks", err).Error(), Cause: err})
					}
				}
			default:
				return report, errors.New("download source is invalid")
			}
			continue
		}
		if id, err := strconv.ParseInt(strings.TrimSpace(source), 10, 64); err == nil && id > 0 {
			ids[id] = true
			continue
		}
		if isDirectResourceURL(source) {
			urls = append(urls, strings.TrimSpace(source))
			continue
		}
		resource, err := parseResourceReference(source)
		if err != nil {
			report.Failures = append(report.Failures, DownloadFailure{URL: redactedDownloadSource, Message: "download source is invalid", Cause: err})
			continue
		}
		refs = append(refs, resource)
	}
	// 2. 同一作品逐页保存，首个失败后停止该作品，已发布文件阻止账号池重放。
	ordered := make([]int64, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	slices.Sort(ordered)
	if len(ordered) > 0 {
		if err := filename.ValidateDirectoryTemplate(request.DirectoryTemplate); err != nil {
			return report, err
		}
	}
	for _, id := range ordered {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		art, err := client.Artwork(id).Get(ctx)
		var item DownloadedArtwork
		if err == nil {
			item, err = saveArtwork(ctx, art, request, &report)
		} else {
			err = downloadError("Artwork", err)
		}
		if len(item.Files) > 0 {
			report.Items = append(report.Items, item)
			report.Committed = true
		}
		if ctx.Err() != nil {
			return report, ctx.Err()
		}
		if err != nil {
			report.Failures = append(report.Failures, DownloadFailure{IllustID: id, Message: err.Error(), Cause: err})
		}
	}
	// 3. 显式资源来源保持独立于作品模板与页选择。
	for _, ref := range refs {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		path, err := directResourcePath(request.DownloadPath, ref)
		if err == nil {
			err = saveReference(ctx, client, ref, path)
		}
		if ctx.Err() != nil {
			return report, ctx.Err()
		}
		if err != nil {
			report.Failures = append(report.Failures, DownloadFailure{URL: ref.Text, Message: err.Error(), Cause: err})
			continue
		}
		report.Items = append(report.Items, DownloadedArtwork{Type: downloadedResourceType, Files: []DownloadedFile{{Path: path, Page: 1}}})
		report.Committed = true
	}
	for _, raw := range urls {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		path, err := directResourcePathFromURL(request.DownloadPath, raw)
		if err == nil {
			var image pixiv.Image
			image, err = client.Image(raw)
			if err == nil {
				err = os.MkdirAll(filepath.Dir(path), downloadDirectoryMode)
			}
			if err == nil {
				err = image.Save(ctx, path)
			}
		}
		if ctx.Err() != nil {
			return report, ctx.Err()
		}
		if err != nil {
			err = downloadError("SaveResourceURL", err)
			report.Failures = append(report.Failures, DownloadFailure{URL: redactedDownloadSource, Type: downloadedResourceType, Message: err.Error(), Cause: err})
			continue
		}
		report.Items = append(report.Items, DownloadedArtwork{Type: downloadedResourceType, Files: []DownloadedFile{{Path: path, Page: 1}}})
		report.Committed = true
	}
	return report, nil
}

func saveArtwork(ctx context.Context, art *pixiv.Artwork, request Request, report *DownloadReport) (DownloadedArtwork, error) {
	options := request.Options
	// 目录按作品展开；文件名的 {num} 仍使用实际页码。
	options.DirectoryTemplate = strings.ReplaceAll(strings.TrimSpace(options.DirectoryTemplate), "{num}", "0")
	animated := art.ContentType == pixiv.ContentTypeUgoira
	if animated {
		if options.Quality != "" && options.Quality != download.QualityOriginal {
			return DownloadedArtwork{}, fmt.Errorf("ugoira quality %q is unsupported; only original is supported", options.Quality)
		}
		if len(request.Pages) > 0 {
			return DownloadedArtwork{}, errors.New("ugoira page selection is unsupported")
		}
	}
	if len(art.Pages) == 0 {
		return DownloadedArtwork{}, fmt.Errorf("illust %d has no page metadata", art.ID)
	}
	// 校验全部页码后才构造下标；错误带回作品实际页数。
	for _, page := range request.Pages {
		if page < 1 || page > len(art.Pages) {
			return DownloadedArtwork{}, fmt.Errorf("page %d does not exist (page_count=%d)", page, art.PageCount)
		}
	}
	selected := art.Pages
	if len(request.Pages) > 0 {
		selected = nil
		for index, page := range art.Pages {
			if slices.Contains(request.Pages, index+1) {
				selected = append(selected, page)
			}
		}
	}
	item := DownloadedArtwork{IllustID: art.ID, Title: art.Title, Author: art.Author.Name, Type: string(art.ContentType)}
	saver := download.New(options)
	for _, page := range selected {
		saved, err := saver.Save(ctx, page)
		for _, warning := range saved.Warnings {
			report.Warnings = append(report.Warnings, DownloadWarning{IllustID: art.ID, Type: string(art.ContentType), Message: warning.Message})
		}
		for _, file := range saved.Files {
			item.Files = append(item.Files, DownloadedFile{Path: file.Path, Page: file.PageIndex + 1})
		}
		if err != nil {
			// SDK 的报告保留原始错误；展示沿用下载命令的错误文本。
			if len(saved.Failures) > 0 {
				err = errors.Unwrap(saved.Failures[0].Cause)
			}
			return item, downloadError("SaveResource", err)
		}
	}
	return item, nil
}

type outputError struct {
	message string
	cause   error
}

func (e outputError) Error() string { return e.message }
func (e outputError) Unwrap() error { return e.cause }

// downloadError 保留分类错误链，同时保持既有命令错误摘要。
func downloadError(operation string, err error) error {
	var failure *sdk.Error
	if !errors.As(err, &failure) {
		return err
	}
	if failure.Operation == "download.Save" && (failure.Reason == sdk.InvalidArgument || failure.Reason == sdk.MalformedUpstreamResponse) {
		return outputError{failure.Message, err}
	}
	text := "pixiv:" + operation + ": " + string(failure.Reason)
	if (operation == "SaveResource" || operation == "SaveResourceURL") && failure.HTTPStatus != 0 {
		text = "pixiv:" + operation + ": upstream_error: resource returned a non-success status"
	}
	return outputError{text, err}
}
