package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/FlanChanXwO/pixiv-cli/internal/download"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/media"
	uriutil "github.com/FlanChanXwO/pixiv-cli/internal/utils/uri"
	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type setDownloadPathIn struct {
	Path string `json:"path" jsonschema:"directory path where files should be downloaded"`
}

func (a *App) setDownloadPath(ctx context.Context, _ *mcp.CallToolRequest, in setDownloadPathIn) (*mcp.CallToolResult, textOut, error) {
	if strings.TrimSpace(in.Path) == "" {
		return toolTextError(ctx, errLegacyValidation, "Error: path must not be empty.")
	}
	if err := a.downloads.SetDownloadPath(in.Path); err != nil {
		return toolTextError(ctx, err, fmt.Sprintf("Error: could not set the download path. Ensure %q is valid and writable. Details: %v", in.Path, err))
	}
	return toolText(fmt.Sprintf("Download path updated: %s. Future downloads will be saved there.", in.Path))
}

type downloadIn struct {
	IllustID  int64    `json:"illust_id,omitempty" jsonschema:"single artwork ID to download"`
	IllustIDs []int64  `json:"illust_ids,omitempty" jsonschema:"artwork IDs to download"`
	URLs      []string `json:"urls,omitempty" jsonschema:"Pixiv artwork or user artworks URLs to download"`
	Pages     string   `json:"pages,omitempty" jsonschema:"1-based page selection, e.g. 1,3-5; default all pages"`
	Quality   string   `json:"quality,omitempty" jsonschema:"static image quality: original, regular, small, thumb, mini"`
	// Delivery 仅保留 local_path 兼容字段；image_content 已移除。
	Delivery string `json:"delivery,omitempty" jsonschema:"delivery mode: local_path only"`
}

type downloadOut struct {
	Delivery string               `json:"delivery"`
	Items    []downloadItemOut    `json:"items"`
	Failures []downloadFailureOut `json:"failures"`
	Files    []downloadFileOut    `json:"files"`
	Text     string               `json:"text"`
}

type downloadItemOut struct {
	URL      string            `json:"url"`
	IllustID int64             `json:"illust_id"`
	Title    string            `json:"title"`
	Author   string            `json:"author"`
	Type     string            `json:"type"`
	Files    []downloadFileOut `json:"files"`
}

type downloadFailureOut struct {
	URL      string `json:"url"`
	IllustID int64  `json:"illust_id"`
	Type     string `json:"type"`
	Message  string `json:"message"`
}

type downloadFileOut struct {
	IllustID  int64  `json:"illust_id"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	Path      string `json:"path"`
	FileURI   string `json:"file_uri"`
	MIMEType  string `json:"mime_type"`
	SizeBytes int64  `json:"size_bytes"`
	Page      int    `json:"page,omitempty"`
}

const (
	deliveryLocalPath = "local_path"
)

func (a *App) download(ctx context.Context, _ *mcp.CallToolRequest, in downloadIn) (*mcp.CallToolResult, downloadOut, error) {
	targets, err := parseDownloadReferences(in)
	if err != nil {
		return emptyDownloadError(ctx, errLegacyValidation, deliveryLocalPath, "Error: "+err.Error())
	}
	delivery, errText := normalizeDelivery(in.Delivery)
	if errText != "" {
		return emptyDownloadError(ctx, errLegacyValidation, deliveryLocalPath, errText)
	}
	pages, quality, err := parseDownloadSelection(in.Pages, in.Quality)
	if err != nil {
		return emptyDownloadError(ctx, err, delivery, "Error: "+err.Error())
	}
	report, err := a.downloadTargets(ctx, targets, pages, quality)
	if err != nil {
		return emptyDownloadError(ctx, err, delivery, "Download failed: "+err.Error())
	}
	out, err := buildDownloadReportOut(delivery, report)
	if err != nil {
		return emptyDownloadError(ctx, err, delivery, "Could not build the download result: "+err.Error())
	}
	// MCP 下载只返回本地 path/file_uri/mime_type/页号/大小，不再内嵌 ImageContent。
	result := downloadResult(out)
	if len(out.Failures) > 0 {
		result.IsError = true
		recordToolError(ctx, errors.New("download completed with failures"))
	}
	return result, out, nil
}

// parseDownloadReferences 保持旧 ID 字段的顺序，并把 URL 按数组顺序追加；MCP 多字段
// 本身没有跨字段顺序，调用方需要精确交错顺序时使用 CLI positional targets。
func parseDownloadReferences(in downloadIn) ([]sdk.Reference, error) {
	targets := make([]sdk.Reference, 0, len(in.IllustIDs)+len(in.URLs)+1)
	for _, id := range in.IllustIDs {
		targets = append(targets, sdk.Reference{Kind: sdk.ReferenceKindArtwork, ID: id})
	}
	if in.IllustID > 0 {
		targets = append(targets, sdk.Reference{Kind: sdk.ReferenceKindArtwork, ID: in.IllustID})
	}
	for _, raw := range in.URLs {
		target, err := sdk.ParseReference(raw)
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	if len(targets) == 0 {
		return nil, errors.New("provide either illust_id (single ID) or illust_ids (list of IDs).")
	}
	return targets, nil
}

func (a *App) downloadTargets(ctx context.Context, targets []sdk.Reference, pages []int, quality application.DownloadQuality) (application.DownloadReport, error) {
	// 旧嵌入构造器没有 SDK runtime，只保留纯 ID 的既有下载路径；URL/作者展开必须
	// 经 public SDK operation 执行，不能伪造匿名或 Cookie 路径。
	if a.newDownloads == nil && a.sdk.NewClient == nil {
		ids := make([]int64, 0, len(targets))
		for _, target := range targets {
			if target.Kind != sdk.ReferenceKindArtwork {
				return application.DownloadReport{}, errors.New("downloading a user URL requires an authenticated Pixiv SDK operation")
			}
			ids = append(ids, target.ID)
		}
		artworks, err := a.downloads.Download(ctx, application.DownloadRequest{IllustIDs: ids, Pages: pages, Quality: quality})
		if err != nil {
			return application.DownloadReport{}, err
		}
		return application.DownloadReport{Items: artworks, Failures: []application.DownloadFailure{}}, nil
	}
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return application.DownloadReport{}, err
	}
	defer release()
	service := application.DownloadService{NewManager: func(_ application.DownloadClient, _, _ string) (application.DownloadManager, error) {
		if a.newDownloads != nil {
			return a.newDownloads(client), nil
		}
		return a.downloads, nil
	}}
	return service.DownloadTargets(ctx, client, targets, application.DownloadRequest{Pages: pages, Quality: quality})
}

func (a *App) downloadArtworks(ctx context.Context, ids []int64, client application.SDKClient, pages []int, quality application.DownloadQuality) ([]download.DownloadedArtwork, error) {
	req := application.DownloadRequest{
		IllustIDs: ids,
		Pages:     pages,
		Quality:   quality,
	}
	if a.newDownloads == nil {
		return a.downloads.Download(ctx, req)
	}
	if client == nil {
		opened, release, err := a.openSDKOperation(ctx)
		if err != nil {
			return nil, err
		}
		defer release()
		client = opened
	}
	return a.newDownloads(client).Download(ctx, req)
}

func parseDownloadSelection(pagesSpec, qualitySpec string) ([]int, application.DownloadQuality, error) {
	pages, err := application.ParsePageSpec(pagesSpec)
	if err != nil {
		return nil, "", err
	}
	quality := application.DownloadQuality(strings.TrimSpace(qualitySpec))
	if quality == "" {
		quality = application.DownloadQualityOriginal
	}
	if err := application.ValidateDownloadQuality(quality); err != nil {
		return nil, "", err
	}
	return pages, quality, nil
}

func downloadResult(out downloadOut) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: out.Text}},
	}
}

func emptyDownloadResult(delivery, text string) (*mcp.CallToolResult, downloadOut, error) {
	out := downloadOut{
		Delivery: delivery,
		Items:    []downloadItemOut{},
		Failures: []downloadFailureOut{},
		Files:    []downloadFileOut{},
		Text:     text,
	}
	return downloadResult(out), out, nil
}

func emptyDownloadError(ctx context.Context, err error, delivery, text string) (*mcp.CallToolResult, downloadOut, error) {
	recordToolError(ctx, err)
	return emptyDownloadResult(delivery, text)
}

func normalizeDelivery(value string) (string, string) {
	switch strings.TrimSpace(value) {
	case "", deliveryLocalPath:
		return deliveryLocalPath, ""
	default:
		return "", fmt.Sprintf("Error: delivery supports only %q.", deliveryLocalPath)
	}
}

func buildDownloadOut(delivery string, artworks []download.DownloadedArtwork) (downloadOut, error) {
	out := downloadOut{Delivery: delivery, Items: []downloadItemOut{}, Failures: []downloadFailureOut{}, Files: []downloadFileOut{}}
	lines := []string{fmt.Sprintf("Download completed; delivery: %s.", delivery)}
	for _, artwork := range artworks {
		item := downloadItemOut{
			URL:      sdk.Reference{Kind: sdk.ReferenceKindArtwork, ID: artwork.IllustID}.URL(),
			IllustID: artwork.IllustID,
			Title:    artwork.Title,
			Author:   artwork.Author,
			Type:     artwork.Type,
			Files:    []downloadFileOut{},
		}
		lines = append(lines, fmt.Sprintf("Artwork %d - %q / Author: %s / Type: %s", artwork.IllustID, artwork.Title, artwork.Author, artwork.Type))
		for _, file := range artwork.Files {
			info, err := os.Stat(file.Path)
			if err != nil {
				return downloadOut{}, err
			}
			mimeType := media.MimeTypeForPath(file.Path)
			fileOut := downloadFileOut{
				IllustID:  artwork.IllustID,
				Title:     artwork.Title,
				Author:    artwork.Author,
				Path:      file.Path,
				FileURI:   uriutil.FileURI(file.Path),
				MIMEType:  mimeType,
				SizeBytes: info.Size(),
				Page:      file.Page,
			}
			item.Files = append(item.Files, fileOut)
			out.Files = append(out.Files, fileOut)
			lines = append(lines, fmt.Sprintf("- %s\n  URI: %s\n  MIME: %s\n  Size: %d bytes", fileOut.Path, fileOut.FileURI, fileOut.MIMEType, fileOut.SizeBytes))
		}
		out.Items = append(out.Items, item)
	}
	out.Text = strings.Join(lines, "\n")
	return out, nil
}

func buildDownloadReportOut(delivery string, report application.DownloadReport) (downloadOut, error) {
	out, err := buildDownloadOut(delivery, report.Items)
	if err != nil {
		return downloadOut{}, err
	}
	for _, failure := range report.Failures {
		out.Failures = append(out.Failures, downloadFailureOut{
			URL: failure.URL, IllustID: failure.IllustID, Type: failure.Type, Message: failure.Message,
		})
		out.Text += fmt.Sprintf("\nFailed %s: %s", failure.URL, failure.Message)
	}
	return out, nil
}

type downloadRandomIn struct {
	Count    *int   `json:"count,omitempty" jsonschema:"optional artwork count; defaults to 5; explicit value must be from 1 to 20"`
	Pages    string `json:"pages,omitempty" jsonschema:"1-based page selection, e.g. 1,3-5; default all pages"`
	Quality  string `json:"quality,omitempty" jsonschema:"static image quality: original, regular, small, thumb, mini"`
	Delivery string `json:"delivery,omitempty" jsonschema:"delivery mode: local_path only"`
}

const (
	downloadRandomDefaultCount = 5
	// 一次请求可触发多个作品下载，每个作品又可展开为多页/多文件，全部产物
	// 元数据会进入同一 structured response；20 约束作品数，避免无界放大下载工作与 JSON-RPC 输出。
	downloadRandomMaxCount = 20
)

var errDownloadRandomCount = errors.New("count must be an integer from 1 to 20")

func parseDownloadRandomCount(value *int) (int, error) {
	if value == nil {
		return downloadRandomDefaultCount, nil
	}
	if *value <= 0 || *value > downloadRandomMaxCount {
		return 0, errDownloadRandomCount
	}
	return *value, nil
}

func (a *App) downloadRandom(ctx context.Context, req *mcp.CallToolRequest, in downloadRandomIn) (*mcp.CallToolResult, downloadOut, error) {
	delivery, errText := normalizeDelivery(in.Delivery)
	if errText != "" {
		return emptyDownloadError(ctx, errLegacyValidation, deliveryLocalPath, errText)
	}
	count, err := parseDownloadRandomCount(in.Count)
	if err != nil {
		return emptyDownloadError(ctx, err, delivery, "Error: "+err.Error())
	}
	pages, quality, err := parseDownloadSelection(in.Pages, in.Quality)
	if err != nil {
		return emptyDownloadError(ctx, err, delivery, "Error: "+err.Error())
	}
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return emptyDownloadError(ctx, err, delivery, "Could not retrieve recommendations: "+err.Error())
	}
	defer release()
	result, err := client.IllustRecommended(ctx, sdk.IllustRecommendedRequest{})
	if err != nil {
		return emptyDownloadError(ctx, err, delivery, "Could not retrieve recommendations: "+err.Error())
	}
	if len(result.Illusts) == 0 {
		return emptyDownloadResult(delivery, "Could not retrieve recommendations: the list is empty.")
	}
	if count > len(result.Illusts) {
		count = len(result.Illusts)
	}
	rand.Shuffle(len(result.Illusts), func(i, j int) { result.Illusts[i], result.Illusts[j] = result.Illusts[j], result.Illusts[i] })
	ids := make([]int64, 0, count)
	for _, illust := range result.Illusts[:count] {
		ids = append(ids, illust.ID)
	}
	artworks, err := a.downloadArtworks(ctx, ids, client, pages, quality)
	if err != nil {
		return emptyDownloadError(ctx, err, delivery, "Download failed: "+err.Error())
	}
	out, err := buildDownloadOut(delivery, artworks)
	if err != nil {
		return emptyDownloadError(ctx, err, delivery, "Could not build the download result: "+err.Error())
	}
	return downloadResult(out), out, nil
}
