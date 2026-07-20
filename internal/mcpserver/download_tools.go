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
		return toolTextError(ctx, errLegacyValidation, "错误：path 不能为空。")
	}
	if err := a.downloads.SetDownloadPath(in.Path); err != nil {
		return toolTextError(ctx, err, fmt.Sprintf("错误：无法设置下载路径。请检查路径 '%s' 是否有效且程序有写入权限。错误详情: %v", in.Path, err))
	}
	return toolText(fmt.Sprintf("下载路径已成功更新为: %s。之后所有下载的文件都将保存于此。", in.Path))
}

type downloadIn struct {
	IllustID  int64   `json:"illust_id,omitempty" jsonschema:"single artwork ID to download"`
	IllustIDs []int64 `json:"illust_ids,omitempty" jsonschema:"artwork IDs to download"`
	Delivery  string  `json:"delivery,omitempty" jsonschema:"delivery mode: local_path or image_content"`
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
	deliveryLocalPath    = "local_path"
	deliveryImageContent = "image_content"
)

func (a *App) download(ctx context.Context, _ *mcp.CallToolRequest, in downloadIn) (*mcp.CallToolResult, downloadOut, error) {
	ids := append([]int64(nil), in.IllustIDs...)
	if in.IllustID > 0 {
		ids = append(ids, in.IllustID)
	}
	if len(ids) == 0 {
		return emptyDownloadError(ctx, errLegacyValidation, deliveryLocalPath, "错误：必须提供 illust_id (单个ID) 或 illust_ids (ID列表) 参数之一。")
	}
	delivery, errText := normalizeDelivery(in.Delivery)
	if errText != "" {
		return emptyDownloadError(ctx, errLegacyValidation, deliveryLocalPath, errText)
	}
	artworks, err := a.downloadArtworks(ctx, ids, nil)
	if err != nil {
		return emptyDownloadError(ctx, err, delivery, "下载失败: "+err.Error())
	}
	out, err := buildDownloadOut(delivery, artworks)
	if err != nil {
		return emptyDownloadError(ctx, err, delivery, "整理下载结果失败: "+err.Error())
	}
	result := downloadResult(out)
	if delivery == deliveryImageContent {
		for _, file := range out.Files {
			data, err := os.ReadFile(file.Path)
			if err != nil {
				return emptyDownloadError(ctx, err, delivery, "读取下载文件失败: "+err.Error())
			}
			result.Content = append(result.Content, &mcp.ImageContent{
				Data:     data,
				MIMEType: file.MIMEType,
			})
		}
	}
	return result, out, nil
}

func (a *App) downloadArtworks(ctx context.Context, ids []int64, client application.SDKClient) ([]download.DownloadedArtwork, error) {
	if a.newDownloads == nil {
		return a.downloads.Download(ctx, application.DownloadRequest{IllustIDs: ids, Quality: application.DownloadQualityOriginal})
	}
	if client == nil {
		opened, release, err := a.openSDKOperation(ctx)
		if err != nil {
			return nil, err
		}
		defer release()
		client = opened
	}
	return a.newDownloads(client).Download(ctx, application.DownloadRequest{IllustIDs: ids, Quality: application.DownloadQualityOriginal})
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
	case deliveryImageContent:
		return deliveryImageContent, ""
	default:
		return "", fmt.Sprintf("错误：delivery 仅支持 %q 或 %q。", deliveryLocalPath, deliveryImageContent)
	}
}

func buildDownloadOut(delivery string, artworks []download.DownloadedArtwork) (downloadOut, error) {
	out := downloadOut{Delivery: delivery, Items: []downloadItemOut{}, Files: []downloadFileOut{}}
	lines := []string{fmt.Sprintf("下载完成，交付方式: %s。", delivery)}
	for _, artwork := range artworks {
		item := downloadItemOut{
			IllustID: artwork.IllustID,
			Title:    artwork.Title,
			Author:   artwork.Author,
			Type:     artwork.Type,
			Files:    []downloadFileOut{},
		}
		lines = append(lines, fmt.Sprintf("作品 %d - %q / 作者: %s / 类型: %s", artwork.IllustID, artwork.Title, artwork.Author, artwork.Type))
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
			lines = append(lines, fmt.Sprintf("- %s\n  URI: %s\n  MIME: %s\n  大小: %d bytes", fileOut.Path, fileOut.FileURI, fileOut.MIMEType, fileOut.SizeBytes))
		}
		out.Items = append(out.Items, item)
	}
	out.Text = strings.Join(lines, "\n")
	return out, nil
}

type downloadRandomIn struct {
	Count    *int   `json:"count,omitempty" jsonschema:"optional artwork count; defaults to 5; explicit value must be from 1 to 20"`
	Delivery string `json:"delivery,omitempty" jsonschema:"delivery mode: local_path or image_content"`
}

const (
	downloadRandomDefaultCount = 5
	// 一次请求可触发多个作品下载，每个作品又可展开为多页/多文件，全部产物
	// 元数据会进入同一 structured response；20 约束作品数，避免无界放大下载工作与 JSON-RPC 输出。
	downloadRandomMaxCount = 20
)

var errDownloadRandomCount = errors.New("count 必须是 1 到 20 之间的整数")

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
		return emptyDownloadError(ctx, err, delivery, "错误："+err.Error()+"。")
	}
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return emptyDownloadError(ctx, err, delivery, "获取推荐列表失败: "+err.Error())
	}
	defer release()
	result, err := client.IllustRecommended(ctx, sdk.IllustRecommendedRequest{})
	if err != nil {
		return emptyDownloadError(ctx, err, delivery, "获取推荐列表失败: "+err.Error())
	}
	if len(result.Illusts) == 0 {
		return emptyDownloadResult(delivery, "无法获取推荐内容，列表为空。")
	}
	if count > len(result.Illusts) {
		count = len(result.Illusts)
	}
	rand.Shuffle(len(result.Illusts), func(i, j int) { result.Illusts[i], result.Illusts[j] = result.Illusts[j], result.Illusts[i] })
	ids := make([]int64, 0, count)
	for _, illust := range result.Illusts[:count] {
		ids = append(ids, illust.ID)
	}
	artworks, err := a.downloadArtworks(ctx, ids, client)
	if err != nil {
		return emptyDownloadError(ctx, err, delivery, "下载失败: "+err.Error())
	}
	out, err := buildDownloadOut(delivery, artworks)
	if err != nil {
		return emptyDownloadError(ctx, err, delivery, "整理下载结果失败: "+err.Error())
	}
	return downloadResult(out), out, nil
}
