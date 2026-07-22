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
	IllustID  int64   `json:"illust_id,omitempty" jsonschema:"single artwork ID to download"`
	IllustIDs []int64 `json:"illust_ids,omitempty" jsonschema:"artwork IDs to download"`
	Pages     string  `json:"pages,omitempty" jsonschema:"1-based page selection, e.g. 1,3-5; default all pages"`
	Quality   string  `json:"quality,omitempty" jsonschema:"static image quality: original, regular, small, thumb, mini"`
	// Delivery 仅保留 local_path 兼容字段；image_content 已移除。
	Delivery string `json:"delivery,omitempty" jsonschema:"delivery mode: local_path only"`
}

type downloadOut struct {
	Delivery string            `json:"delivery"`
	Items    []downloadItemOut `json:"items"`
	Files    []downloadFileOut `json:"files"`
	Text     string            `json:"text"`
}

type downloadItemOut struct {
	IllustID int64             `json:"illust_id"`
	Title    string            `json:"title"`
	Author   string            `json:"author"`
	Type     string            `json:"type"`
	Files    []downloadFileOut `json:"files"`
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
	ids := append([]int64(nil), in.IllustIDs...)
	if in.IllustID > 0 {
		ids = append(ids, in.IllustID)
	}
	if len(ids) == 0 {
		return emptyDownloadError(ctx, errLegacyValidation, deliveryLocalPath, "Error: provide either illust_id (single ID) or illust_ids (list of IDs).")
	}
	delivery, errText := normalizeDelivery(in.Delivery)
	if errText != "" {
		return emptyDownloadError(ctx, errLegacyValidation, deliveryLocalPath, errText)
	}
	pages, quality, err := parseDownloadSelection(in.Pages, in.Quality)
	if err != nil {
		return emptyDownloadError(ctx, err, delivery, "Error: "+err.Error())
	}
	artworks, err := a.downloadArtworks(ctx, ids, nil, pages, quality)
	if err != nil {
		return emptyDownloadError(ctx, err, delivery, "Download failed: "+err.Error())
	}
	out, err := buildDownloadOut(delivery, artworks)
	if err != nil {
		return emptyDownloadError(ctx, err, delivery, "Could not build the download result: "+err.Error())
	}
	// MCP 下载只返回本地 path/file_uri/mime_type/页号/大小，不再内嵌 ImageContent。
	return downloadResult(out), out, nil
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
	out := downloadOut{Delivery: delivery, Items: []downloadItemOut{}, Files: []downloadFileOut{}}
	lines := []string{fmt.Sprintf("Download completed; delivery: %s.", delivery)}
	for _, artwork := range artworks {
		item := downloadItemOut{
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
