// Package download 实现 download 与 download_random_from_recommendation tool。
package download

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime"
	downloader "github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	uriutil "github.com/FlanChanXwO/pixiv-cli/internal/utils/uri"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var errInputValidation = errors.New("tool input validation failed")

// Register 注册 download 与 download_random_from_recommendation。
func Register(app *runtime.App, server *mcp.Server) {
	runtime.AddTool(app, server, &mcp.Tool{Name: "download", Description: "Download artwork IDs or supported Pixiv artwork/user URLs with intelligent storage rules."}, func(ctx context.Context, request *mcp.CallToolRequest, input downloadIn) (*mcp.CallToolResult, downloadOut, error) {
		return handleDownload(ctx, app, input)
	})
	runtime.AddTool(app, server, &mcp.Tool{Name: "download_random_from_recommendation", Description: "Download random artworks from recommendations."}, func(ctx context.Context, request *mcp.CallToolRequest, input downloadRandomIn) (*mcp.CallToolResult, downloadOut, error) {
		return handleDownloadRandom(ctx, app, input)
	})
}

type downloadIn struct {
	Src        string   `json:"src,omitempty" jsonschema:"PID, Pixiv artwork/user URL, or allowed CDN resource URL"`
	Srcs       []string `json:"srcs,omitempty" jsonschema:"multiple PID, Pixiv artwork/user URL, or allowed CDN resource URLs"`
	Pages      string   `json:"pages,omitempty" jsonschema:"1-based page selection, e.g. 1,3-5; default all pages"`
	Quality    string   `json:"quality,omitempty" jsonschema:"static image quality: original, regular, small, thumb, mini"`
	UgoiraMode string   `json:"ugoira_mode,omitempty" jsonschema:"ugoira output mode: gif or apng; default gif"`
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

func handleDownload(ctx context.Context, app *runtime.App, in downloadIn) (*mcp.CallToolResult, downloadOut, error) {
	sources, err := parseDownloadSources(in)
	if err != nil {
		return emptyDownloadError(ctx, errInputValidation, deliveryLocalPath, "Error: "+err.Error())
	}
	delivery, errText := normalizeDelivery(in.Delivery)
	if errText != "" {
		return emptyDownloadError(ctx, errInputValidation, deliveryLocalPath, errText)
	}
	pages, quality, ugoiraFormat, err := ParseDownloadOptions(in.Pages, in.Quality, in.UgoiraMode)
	if err != nil {
		return emptyDownloadError(ctx, err, delivery, "Error: "+err.Error())
	}
	report, err := downloadSources(ctx, app, sources, pages, quality, ugoiraFormat)
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
	}
	return result, out, nil
}

func parseDownloadSources(in downloadIn) ([]string, error) {
	if strings.TrimSpace(in.Src) != "" && len(in.Srcs) > 0 {
		return nil, errors.New("provide src or srcs, not both")
	}
	if strings.TrimSpace(in.Src) != "" {
		return []string{in.Src}, nil
	}
	if len(in.Srcs) == 0 {
		return nil, errors.New("provide src (one source) or srcs (a source list)")
	}
	return append([]string(nil), in.Srcs...), nil
}

// downloadSources 把 MCP 下载请求交给 downloader.DownloadService：PID、作品/用户
// URL 与受资源策略允许的直链都经 public SDK 解析，统一走 DownloadSources 编排。
func downloadSources(ctx context.Context, app *runtime.App, sources []string, pages []int, quality downloader.DownloadQuality, ugoiraFormat downloader.UgoiraFormat) (downloader.DownloadReport, error) {
	lease, err := app.OpenClient(ctx)
	if err != nil {
		return downloader.DownloadReport{}, err
	}
	defer lease.Close()
	client := lease.Value()
	service := downloader.DownloadService{NewManager: func(_ downloader.DownloadClient, _, _ string) (downloader.DownloadManager, error) {
		if newDownloads := app.NewDownloads(); newDownloads != nil {
			return newDownloads(client), nil
		}
		return app.Downloads(), nil
	}}
	path, template := downloadDefaults(app)
	request := downloader.DownloadRequest{
		Pages:            pages,
		Quality:          quality,
		UgoiraFormat:     ugoiraFormat,
		DownloadPath:     path,
		FilenameTemplate: template,
	}
	return service.DownloadSources(ctx, client, sources, request)
}

func downloadDefaults(app *runtime.App) (string, string) {
	downloads := app.Downloads()
	path, template := "", ""
	if configured, ok := downloads.(interface{ DownloadPath() string }); ok && strings.TrimSpace(configured.DownloadPath()) != "" {
		path = configured.DownloadPath()
	}
	if configured, ok := downloads.(interface{ FilenameTemplate() string }); ok && strings.TrimSpace(configured.FilenameTemplate()) != "" {
		template = configured.FilenameTemplate()
	}
	return path, template
}

func downloadArtworks(ctx context.Context, app *runtime.App, ids []int64, client *pixiv.Client, pages []int, quality downloader.DownloadQuality, ugoiraFormat downloader.UgoiraFormat) ([]downloader.DownloadedArtwork, error) {
	req := downloader.DownloadRequest{
		IllustIDs:    ids,
		Pages:        pages,
		Quality:      quality,
		UgoiraFormat: ugoiraFormat,
	}
	newDownloads := app.NewDownloads()
	if newDownloads == nil {
		return app.Downloads().Download(ctx, req)
	}
	if client == nil {
		lease, err := app.OpenClient(ctx)
		if err != nil {
			return nil, err
		}
		defer lease.Close()
		client = lease.Value()
	}
	return newDownloads(client).Download(ctx, req)
}

func ParseDownloadSelection(pagesSpec, qualitySpec, ugoiraFormatSpec string) ([]int, downloader.DownloadQuality, downloader.UgoiraFormat, error) {
	pages, err := downloader.ParsePageSpec(pagesSpec)
	if err != nil {
		return nil, "", "", err
	}
	quality := downloader.DownloadQuality(strings.TrimSpace(qualitySpec))
	if quality == "" {
		quality = downloader.DownloadQualityOriginal
	}
	if err := downloader.ValidateDownloadQuality(quality); err != nil {
		return nil, "", "", err
	}
	ugoiraFormat := downloader.UgoiraFormat(strings.TrimSpace(ugoiraFormatSpec))
	if ugoiraFormat == "" {
		ugoiraFormat = downloader.UgoiraFormatGIF
	}
	if err := downloader.ValidateUgoiraFormat(ugoiraFormat); err != nil {
		return nil, "", "", err
	}
	return pages, quality, ugoiraFormat, nil
}

// ParseDownloadOptions 解析主 MCP download tool 的当前契约：页码选择、静态质量与
// ugoira 容器格式统一由 application 解析与校验。
func ParseDownloadOptions(pagesSpec, qualitySpec, ugoiraModeSpec string) ([]int, downloader.DownloadQuality, downloader.UgoiraFormat, error) {
	pages, err := downloader.ParsePageSpec(pagesSpec)
	if err != nil {
		return nil, "", "", err
	}
	quality := downloader.DownloadQuality(strings.TrimSpace(qualitySpec))
	if quality == "" {
		quality = downloader.DownloadQualityOriginal
	}
	if err := downloader.ValidateDownloadQuality(quality); err != nil {
		return nil, "", "", err
	}
	ugoiraFormat := downloader.UgoiraFormat(strings.TrimSpace(ugoiraModeSpec))
	if ugoiraFormat == "" {
		ugoiraFormat = downloader.UgoiraFormatGIF
	}
	if err := downloader.ValidateUgoiraFormat(ugoiraFormat); err != nil {
		return nil, "", "", err
	}
	return pages, quality, ugoiraFormat, nil
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

func emptyDownloadError(_ context.Context, _ error, delivery, text string) (*mcp.CallToolResult, downloadOut, error) {
	result, out, resultErr := emptyDownloadResult(delivery, text)
	result.IsError = true
	return result, out, resultErr
}

func normalizeDelivery(value string) (string, string) {
	switch strings.TrimSpace(value) {
	case "", deliveryLocalPath:
		return deliveryLocalPath, ""
	default:
		return "", fmt.Sprintf("Error: delivery supports only %q.", deliveryLocalPath)
	}
}

func buildDownloadOut(delivery string, artworks []downloader.DownloadedArtwork) (downloadOut, error) {
	out := downloadOut{Delivery: delivery, Items: []downloadItemOut{}, Failures: []downloadFailureOut{}, Files: []downloadFileOut{}}
	lines := []string{fmt.Sprintf("Download completed; delivery: %s.", delivery)}
	for _, artwork := range artworks {
		item := downloadItemOut{
			URL:      "https://www.pixiv.net/artworks/" + strconv.FormatInt(artwork.IllustID, 10),
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
			mimeType := downloader.MimeTypeForPath(file.Path)
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

func buildDownloadReportOut(delivery string, report downloader.DownloadReport) (downloadOut, error) {
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
	Count      *int   `json:"count,omitempty" jsonschema:"optional artwork count; defaults to 5; explicit value must be from 1 to 20"`
	Pages      string `json:"pages,omitempty" jsonschema:"1-based page selection, e.g. 1,3-5; default all pages"`
	Quality    string `json:"quality,omitempty" jsonschema:"static image quality: original, regular, small, thumb, mini"`
	UgoiraMode string `json:"ugoira_mode,omitempty" jsonschema:"ugoira output mode; default gif"`
	Delivery   string `json:"delivery,omitempty" jsonschema:"delivery mode: local_path only"`
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

func handleDownloadRandom(ctx context.Context, app *runtime.App, in downloadRandomIn) (*mcp.CallToolResult, downloadOut, error) {
	delivery, errText := normalizeDelivery(in.Delivery)
	if errText != "" {
		return emptyDownloadError(ctx, errInputValidation, deliveryLocalPath, errText)
	}
	count, err := parseDownloadRandomCount(in.Count)
	if err != nil {
		return emptyDownloadError(ctx, err, delivery, "Error: "+err.Error())
	}
	pages, quality, ugoiraFormat, err := ParseDownloadSelection(in.Pages, in.Quality, in.UgoiraMode)
	if err != nil {
		return emptyDownloadError(ctx, err, delivery, "Error: "+err.Error())
	}
	lease, err := app.OpenClient(ctx)
	if err != nil {
		return emptyDownloadError(ctx, err, delivery, "Could not retrieve recommendations: "+err.Error())
	}
	defer lease.Close()
	client := lease.Value()
	result, err := client.RecommendedArtworks(ctx, pixiv.RecommendedArtworksRequest{})
	if err != nil {
		return emptyDownloadError(ctx, err, delivery, "Could not retrieve recommendations: "+err.Error())
	}
	if len(result.Items) == 0 {
		return emptyDownloadError(ctx, errors.New("recommended illustration list is empty"), delivery, "Could not retrieve recommendations: the list is empty.")
	}
	if count > len(result.Items) {
		count = len(result.Items)
	}
	rand.Shuffle(len(result.Items), func(i, j int) { result.Items[i], result.Items[j] = result.Items[j], result.Items[i] })
	ids := make([]int64, 0, count)
	for _, illust := range result.Items[:count] {
		ids = append(ids, illust.ID)
	}
	artworks, err := downloadArtworks(ctx, app, ids, client, pages, quality, ugoiraFormat)
	if err != nil {
		return emptyDownloadError(ctx, err, delivery, "Download failed: "+err.Error())
	}
	out, err := buildDownloadOut(delivery, artworks)
	if err != nil {
		return emptyDownloadError(ctx, err, delivery, "Could not build the download result: "+err.Error())
	}
	return downloadResult(out), out, nil
}
