package mcpserver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/application"
	"github.com/FlanChanXwO/pixiv-cli/internal/download"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/media"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/text"
	uriutil "github.com/FlanChanXwO/pixiv-cli/internal/utils/uri"
	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type DownloadManager interface {
	SetDownloadPath(string) error
	Download(context.Context, []int64) ([]download.DownloadedArtwork, error)
}

type App struct {
	downloads DownloadManager
	// newDownloads 在每个 SDK operation 的稳定认证 snapshot 上创建下载器。
	// 固定 downloads 仅保留给未注入 SDK 的嵌入测试兼容路径。
	newDownloads func(application.SDKClient) DownloadManager
	logger       *slog.Logger
	sdk          application.SDKService
	sdkRequest   application.SDKClientRequest
	// sdkGate 串行化同一 MCP 实例中会刷新 OAuth 的 SDK operation，避免两个
	// OpenDefault 快照同时消费同一个 rotation token；channel select 可响应 ctx。
	sdkGate chan struct{}
}

// toolErrorCapture 将 handler 已转换为 MCP error result 的原始安全错误交给注册层。
// go-sdk 对 result 与 error 同时存在时会重包 result，因此不能直接返回 handler error。
type toolErrorCapture struct{ err error }

type toolErrorCaptureKey struct{}

func recordToolError(ctx context.Context, err error) {
	if err == nil {
		return
	}
	capture, _ := ctx.Value(toolErrorCaptureKey{}).(*toolErrorCapture)
	if capture != nil && capture.err == nil {
		capture.err = err
	}
}

// New 保留构造参数位置以便嵌入方平滑升级；第一个参数不再被读取，所有 Pixiv
// 能力必须由 public SDK service 提供。
func New(_ any, downloads DownloadManager, logger *slog.Logger) *mcp.Server {
	logger = loggerOrDiscard(logger)
	return newServer(&App{downloads: downloads, logger: logger})
}

// NewWithSDK 通过公共 SDK 为每个 MCP tool 建立独立 operation snapshot。
// 首个参数仅是已废弃的兼容占位，绝不构成内容、认证或资源调用链。
func NewWithSDK(_ any, downloads DownloadManager, logger *slog.Logger, service application.SDKService, request application.SDKClientRequest) *mcp.Server {
	logger = loggerOrDiscard(logger)
	return newServer(&App{downloads: downloads, logger: logger, sdk: service, sdkRequest: request})
}

// NewWithSDKDownloadFactory 为生产 MCP 注入 snapshot-scoped 下载器构造器。
func NewWithSDKDownloadFactory(downloads DownloadManager, newDownloads func(application.SDKClient) DownloadManager, logger *slog.Logger, service application.SDKService, request application.SDKClientRequest) *mcp.Server {
	logger = loggerOrDiscard(logger)
	return newServer(&App{downloads: downloads, newDownloads: newDownloads, logger: logger, sdk: service, sdkRequest: request})
}

func loggerOrDiscard(logger *slog.Logger) *slog.Logger {
	if logger != nil {
		return logger
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newServer(app *App) *mcp.Server {
	if app.sdkGate == nil {
		app.sdkGate = make(chan struct{}, 1)
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "pixiv-cli", Version: "2.0.0"}, &mcp.ServerOptions{
		Instructions: "Pixiv MCP server for searching, browsing, and downloading Pixiv content.",
	})
	app.register(server)
	return server
}

// addTool 在注册层统一观测所有 MCP tool（包括旧兼容 tool）。wrapper 不读取
// request arguments，避免 token、cookie、URL 或用户查询进入日志；日志仍只经注入的
// stderr logger 输出，绝不触碰 JSON-RPC transport。
func addTool[In, Out any](a *App, server *mcp.Server, tool *mcp.Tool, handler mcp.ToolHandlerFor[In, Out]) {
	mcp.AddTool(server, tool, func(ctx context.Context, request *mcp.CallToolRequest, input In) (result *mcp.CallToolResult, output Out, err error) {
		started := time.Now()
		capture := &toolErrorCapture{}
		result, output, err = handler(context.WithValue(ctx, toolErrorCaptureKey{}, capture), request, input)
		logErr := err
		resultError := result != nil && result.IsError
		if resultError && err == nil {
			// 仅供日志标记失败。把它作为 handler 返回 error 会让 go-sdk 重新包装
			// CallToolResult，丢失调用方已有的 Content/StructuredContent。
			logErr = capture.err
		}
		a.operationLog(tool.Name, started, resultError || err != nil, logErr, 0, 0)
		return result, output, err
	})
}

func (a *App) register(server *mcp.Server) {
	addTool(a, server, &mcp.Tool{Name: "set_download_path", Description: "Set the default local save location for images and animations."}, a.setDownloadPath)
	addTool(a, server, &mcp.Tool{Name: "download", Description: "Download one or more artworks by ID with intelligent storage rules."}, a.download)
	addTool(a, server, &mcp.Tool{Name: "refresh_token", Description: "Manually refresh Pixiv API token when encountering authentication errors."}, a.refreshToken)
	addTool(a, server, &mcp.Tool{Name: "set_refresh_token", Description: "Set or update the Pixiv refresh token for authentication."}, a.setRefreshToken)
	addTool(a, server, &mcp.Tool{Name: "download_random_from_recommendation", Description: "Download random artworks from recommendations."}, a.downloadRandom)
	addTool(a, server, &mcp.Tool{Name: "search_illust", Description: "Search for illustrations using keywords with filters."}, a.searchIllust)
	addTool(a, server, &mcp.Tool{Name: "illust_detail", Description: "Get detailed information about a specific artwork."}, a.illustDetail)
	addTool(a, server, &mcp.Tool{Name: "illust_related", Description: "Find artworks related to a specific illustration."}, a.illustRelated)
	addTool(a, server, &mcp.Tool{Name: "illust_ranking", Description: "Browse Pixiv rankings."}, a.illustRanking)
	addTool(a, server, &mcp.Tool{Name: "search_user", Description: "Search for users/artists on Pixiv."}, a.searchUser)
	addTool(a, server, &mcp.Tool{Name: "illust_recommended", Description: "Get personalized artwork recommendations."}, a.illustRecommended)
	addTool(a, server, &mcp.Tool{Name: "recommended", Description: "Get typed personalized recommendations through the Pixiv SDK."}, a.recommended)
	addTool(a, server, &mcp.Tool{Name: "trending_tags_illust", Description: "Get currently trending illustration tags."}, a.trendingTags)
	addTool(a, server, &mcp.Tool{Name: "illust_follow", Description: "Browse artworks from followed artists."}, a.illustFollow)
	addTool(a, server, &mcp.Tool{Name: "user_detail", Description: "Get a user's complete profile through the authenticated Pixiv SDK."}, a.userDetail)
	addTool(a, server, &mcp.Tool{Name: "user_artworks", Description: "Browse a user's artworks."}, a.userArtworks)
	addTool(a, server, &mcp.Tool{Name: "user_bookmarks", Description: "Browse user's bookmarked artworks."}, a.userBookmarks)
	addTool(a, server, &mcp.Tool{Name: "user_following", Description: "View user's following list."}, a.userFollowing)
	addTool(a, server, &mcp.Tool{Name: "add_bookmark", Description: "Add an artwork to bookmarks."}, a.addBookmark)
	addTool(a, server, &mcp.Tool{Name: "remove_bookmark", Description: "Remove an artwork from bookmarks."}, a.removeBookmark)
	addTool(a, server, &mcp.Tool{Name: "follow_user", Description: "Follow a Pixiv user."}, a.followUser)
	addTool(a, server, &mcp.Tool{Name: "unfollow_user", Description: "Unfollow a Pixiv user."}, a.unfollowUser)
	addTool(a, server, &mcp.Tool{Name: "get_thumbnail_base64", Description: "Get artwork thumbnail as base64 data URL."}, a.thumbnailBase64)
}

type textOut struct {
	Text string `json:"text"`
}

func toolText(text string) (*mcp.CallToolResult, textOut, error) {
	return nil, textOut{Text: text}, nil
}

type setDownloadPathIn struct {
	Path string `json:"path" jsonschema:"directory path where files should be downloaded"`
}

func (a *App) setDownloadPath(ctx context.Context, _ *mcp.CallToolRequest, in setDownloadPathIn) (*mcp.CallToolResult, textOut, error) {
	if strings.TrimSpace(in.Path) == "" {
		return toolText("错误：path 不能为空。")
	}
	if err := a.downloads.SetDownloadPath(in.Path); err != nil {
		return toolText(fmt.Sprintf("错误：无法设置下载路径。请检查路径 '%s' 是否有效且程序有写入权限。错误详情: %v", in.Path, err))
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
		return emptyDownloadResult(deliveryLocalPath, "错误：必须提供 illust_id (单个ID) 或 illust_ids (ID列表) 参数之一。")
	}
	delivery, errText := normalizeDelivery(in.Delivery)
	if errText != "" {
		return emptyDownloadResult(deliveryLocalPath, errText)
	}
	artworks, err := a.downloadArtworks(ctx, ids, nil)
	if err != nil {
		return emptyDownloadResult(delivery, "下载失败: "+err.Error())
	}
	out, err := buildDownloadOut(delivery, artworks)
	if err != nil {
		return emptyDownloadResult(delivery, "整理下载结果失败: "+err.Error())
	}
	result := downloadResult(out)
	if delivery == deliveryImageContent {
		for _, file := range out.Files {
			data, err := os.ReadFile(file.Path)
			if err != nil {
				return emptyDownloadResult(delivery, "读取下载文件失败: "+err.Error())
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
		return a.downloads.Download(ctx, ids)
	}
	if client == nil {
		opened, release, err := a.openSDKOperation(ctx)
		if err != nil {
			return nil, err
		}
		defer release()
		client = opened
	}
	return a.newDownloads(client).Download(ctx, ids)
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

type emptyIn struct{}

func (a *App) refreshToken(ctx context.Context, _ *mcp.CallToolRequest, _ emptyIn) (*mcp.CallToolResult, textOut, error) {
	client, release, err := a.openSDKMutable(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return toolText("Token刷新已取消。")
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return toolText("Token刷新失败：操作超时。")
		}
		var sdkErr *sdk.Error
		if errors.As(err, &sdkErr) {
			return toolText("Token刷新失败：无法初始化 Pixiv SDK：" + sdkErr.Error() + "。")
		}
		return toolText("Token刷新失败：无法初始化 Pixiv SDK。请检查本地配置或代理设置。")
	}
	defer release()
	account, err := client.Refresh(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return toolText("Token刷新已取消。")
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return toolText("Token刷新失败：操作超时。")
		}
		if errors.Is(err, sdk.ErrUnauthorized) {
			return toolText("错误：未设置 refresh token。请先使用 set_refresh_token 工具设置 token。")
		}
		var sdkErr *sdk.Error
		if errors.As(err, &sdkErr) {
			return toolText("Token刷新失败：" + sdkErr.Error() + "。")
		}
		return toolText("Token刷新失败。请检查 refresh token 是否有效，以及网络连接或代理设置。")
	}
	a.sdkRequest.UserID = account.UserID
	return toolText(fmt.Sprintf("Token刷新成功！%s。现在可以正常使用Pixiv API功能了。", authIdentityText(*account)))
}

type setRefreshTokenIn struct {
	RefreshToken string `json:"refresh_token" jsonschema:"Pixiv refresh token"`
}

func (a *App) setRefreshToken(ctx context.Context, _ *mcp.CallToolRequest, in setRefreshTokenIn) (*mcp.CallToolResult, textOut, error) {
	token, err := utils.ValidateRefreshTokenInput(in.RefreshToken)
	if err != nil {
		return toolText("错误：" + err.Error())
	}
	if token == "" {
		return toolText("错误：refresh token 不能为空。")
	}
	client, release, err := a.openSDKMutable(ctx)
	if err != nil {
		return toolText("Refresh token 已在当前会话设置，但认证失败: " + err.Error())
	}
	defer release()
	account, err := client.ImportAccount(ctx, token)
	if err != nil {
		return toolText(fmt.Sprintf("Refresh token 已在当前会话设置，但认证失败: %v\n\n请检查 token 是否有效，或稍后使用 refresh_token 工具重试认证。", err))
	}
	if err := client.SelectAccount(account.UserID); err != nil {
		return toolText("Refresh token 已在当前会话设置并完成认证，但无法选择认证账号: " + err.Error())
	}
	a.sdkRequest.UserID = account.UserID
	return toolText(fmt.Sprintf("Refresh token 已在当前会话设置并完成认证！\n%s\n\n现在您可以使用所有 Pixiv 功能了。", authIdentityText(*account)))
}

func authIdentityText(account sdk.Account) string {
	identity := fmt.Sprintf("用户 ID: %d", account.UserID)
	if username := strings.TrimSpace(account.Username); username != "" {
		identity += "\n用户名: " + username
	}
	return identity
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
		return emptyDownloadResult(deliveryLocalPath, errText)
	}
	count, err := parseDownloadRandomCount(in.Count)
	if err != nil {
		return emptyDownloadResult(delivery, "错误："+err.Error()+"。")
	}
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return emptyDownloadResult(delivery, "获取推荐列表失败: "+err.Error())
	}
	defer release()
	result, err := client.IllustRecommended(ctx, sdk.IllustRecommendedRequest{})
	if err != nil {
		return emptyDownloadResult(delivery, "获取推荐列表失败: "+err.Error())
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
		return emptyDownloadResult(delivery, "下载失败: "+err.Error())
	}
	out, err := buildDownloadOut(delivery, artworks)
	if err != nil {
		return emptyDownloadResult(delivery, "整理下载结果失败: "+err.Error())
	}
	return downloadResult(out), out, nil
}

type searchIllustIn struct {
	Word             string `json:"word"`
	SearchTarget     string `json:"search_target,omitempty"`
	Sort             string `json:"sort,omitempty"`
	Duration         string `json:"duration,omitempty"`
	Offset           int    `json:"offset,omitempty"`
	SearchR18        bool   `json:"search_r18,omitempty"`
	IncludeThumbnail bool   `json:"include_thumbnail,omitempty"`
}

func (a *App) searchIllust(ctx context.Context, _ *mcp.CallToolRequest, in searchIllustIn) (*mcp.CallToolResult, textOut, error) {
	if in.SearchTarget == "" {
		in.SearchTarget = string(sdk.SearchTargetPartialMatchForTags)
	}
	if in.Sort == "" {
		in.Sort = string(sdk.SortModeDateDesc)
	}
	word := in.Word
	if in.SearchR18 {
		word += " R-18"
	}
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return toolText(err.Error())
	}
	defer release()
	items, _, err := collectPages(ctx, offsetPlan(in.Offset), func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
		result, err := client.SearchIllust(ctx, sdk.SearchIllustRequest{Word: word, Target: sdk.SearchTarget(in.SearchTarget), Sort: sdk.SortMode(in.Sort), Duration: in.Duration, Cursor: cursor})
		if err != nil {
			return nil, "", err
		}
		return result.Illusts, result.NextCursor, nil
	})
	if err != nil {
		return toolText(err.Error())
	}
	if len(items) == 0 {
		return toolText(fmt.Sprintf("抱歉，根据您提供的关键词 '%s'，未能找到相关的插画。", word))
	}
	return toolText(fmt.Sprintf("找到 %d 张关于 '%s' 的插画:\n\n%s", len(items), word, formatIllusts(items, in.IncludeThumbnail, in.Offset, false)))
}

type illustIDIn struct {
	IllustID int64 `json:"illust_id"`
}

func (a *App) illustDetail(ctx context.Context, _ *mcp.CallToolRequest, in illustIDIn) (*mcp.CallToolResult, textOut, error) {
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return toolText(err.Error())
	}
	defer release()
	result, err := client.IllustDetail(ctx, in.IllustID)
	if err != nil {
		return toolText(err.Error())
	}
	body, _ := json.MarshalIndent(result.Illust, "", "  ")
	return toolText(string(body))
}

type relatedIn struct {
	IllustID         int64 `json:"illust_id"`
	Offset           int   `json:"offset,omitempty"`
	IncludeThumbnail bool  `json:"include_thumbnail,omitempty"`
}

func (a *App) illustRelated(ctx context.Context, _ *mcp.CallToolRequest, in relatedIn) (*mcp.CallToolResult, textOut, error) {
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return toolText(err.Error())
	}
	defer release()
	items, _, err := collectPages(ctx, offsetPlan(in.Offset), func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
		result, err := client.IllustRelated(ctx, sdk.IllustRelatedRequest{IllustID: in.IllustID, Cursor: cursor})
		if err != nil {
			return nil, "", err
		}
		return result.Illusts, result.NextCursor, nil
	})
	if err != nil {
		return toolText(err.Error())
	}
	if len(items) == 0 {
		return toolText(fmt.Sprintf("找不到与插画 %d 相关的推荐。", in.IllustID))
	}
	return toolText(fmt.Sprintf("找到 %d 张相关推荐:\n\n%s", len(items), formatIllusts(items, in.IncludeThumbnail, in.Offset, false)))
}

type rankingIn struct {
	Mode             string `json:"mode,omitempty"`
	Date             string `json:"date,omitempty"`
	Offset           int    `json:"offset,omitempty"`
	IncludeThumbnail bool   `json:"include_thumbnail,omitempty"`
}

var rankingLabels = map[sdk.RankingMode]string{
	sdk.RankingModeDay:          "每日排行榜",
	sdk.RankingModeDayMale:      "男性向每日排行榜",
	sdk.RankingModeDayFemale:    "女性向每日排行榜",
	sdk.RankingModeWeek:         "每周排行榜",
	sdk.RankingModeWeekOriginal: "原创作品排行榜",
	sdk.RankingModeWeekRookie:   "新人排行榜",
	sdk.RankingModeMonth:        "每月排行榜",
}

func rankingLabel(mode string) string {
	if label, ok := rankingLabels[sdk.RankingMode(mode)]; ok {
		return label
	}
	return mode + " 排行榜"
}

func (a *App) illustRanking(ctx context.Context, _ *mcp.CallToolRequest, in rankingIn) (*mcp.CallToolResult, textOut, error) {
	if in.Mode == "" {
		in.Mode = string(sdk.RankingModeDay)
	}
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return toolText(err.Error())
	}
	defer release()
	items, _, err := collectPages(ctx, offsetPlan(in.Offset), func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
		result, err := client.IllustRanking(ctx, sdk.IllustRankingRequest{Mode: sdk.RankingMode(in.Mode), Date: in.Date, Cursor: cursor})
		if err != nil {
			return nil, "", err
		}
		return result.Illusts, result.NextCursor, nil
	})
	if err != nil {
		return toolText(err.Error())
	}
	if len(items) == 0 {
		return toolText(fmt.Sprintf("找不到模式为 '%s' 的排行榜结果。", in.Mode))
	}
	return toolText(fmt.Sprintf("%s:\n\n%s", rankingLabel(in.Mode), formatIllusts(items, in.IncludeThumbnail, in.Offset, true)))
}

type searchUserIn struct {
	Word   string `json:"word"`
	Offset int    `json:"offset,omitempty"`
}

func (a *App) searchUser(ctx context.Context, _ *mcp.CallToolRequest, in searchUserIn) (*mcp.CallToolResult, textOut, error) {
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return toolText(err.Error())
	}
	defer release()
	items, _, err := collectPages(ctx, offsetPlan(in.Offset), func(ctx context.Context, cursor sdk.Cursor) ([]sdk.UserPreview, sdk.Cursor, error) {
		result, err := client.SearchUser(ctx, sdk.SearchUserRequest{Word: in.Word, Cursor: cursor})
		if err != nil {
			return nil, "", err
		}
		return result.UserPreviews, result.NextCursor, nil
	})
	if err != nil {
		return toolText(err.Error())
	}
	if len(items) == 0 {
		return toolText(fmt.Sprintf("抱歉，未能找到名为 '%s' 的用户。", in.Word))
	}
	return toolText(fmt.Sprintf("找到 %d 位用户:\n\n%s", len(items), formatUsers(items)))
}

// offsetPlan 将 legacy offset 映射为 SDK cursor 的逻辑跳过。它保留旧工具“从
// offset 位置取一批”的可观察选择，不把已废弃上游 offset 泄露给公开 SDK。
func offsetPlan(offset int) mcpListPlan {
	if offset < 0 {
		offset = 0
	}
	return mcpListPlan{page: 1, limit: -1, oneBatch: true, skip: offset}
}

type offsetThumbnailIn struct {
	Offset           int  `json:"offset,omitempty"`
	IncludeThumbnail bool `json:"include_thumbnail,omitempty"`
}

func (a *App) illustRecommended(ctx context.Context, _ *mcp.CallToolRequest, in offsetThumbnailIn) (*mcp.CallToolResult, textOut, error) {
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return toolText(err.Error())
	}
	defer release()
	items, _, err := collectPages(ctx, offsetPlan(in.Offset), func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
		result, err := client.IllustRecommended(ctx, sdk.IllustRecommendedRequest{Cursor: cursor})
		if err != nil {
			return nil, "", err
		}
		return result.Illusts, result.NextCursor, nil
	})
	if err != nil {
		return toolText(err.Error())
	}
	if len(items) == 0 {
		return toolText("暂无推荐内容。")
	}
	return toolText(fmt.Sprintf("为您推荐 %d 张插画:\n\n%s", len(items), formatIllusts(items, in.IncludeThumbnail, in.Offset, false)))
}

func (a *App) trendingTags(ctx context.Context, _ *mcp.CallToolRequest, _ emptyIn) (*mcp.CallToolResult, textOut, error) {
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return toolText(err.Error())
	}
	defer release()
	result, err := client.TrendingTagsIllust(ctx)
	if err != nil {
		return toolText(err.Error())
	}
	if len(result.TrendTags) == 0 {
		return toolText("无法获取热门标签。")
	}
	lines := make([]string, 0, len(result.TrendTags))
	for _, tag := range result.TrendTags {
		translated := tag.TranslatedName
		if translated == "" {
			translated = "无"
		}
		lines = append(lines, fmt.Sprintf("- %s (翻译: %s)", tag.Tag, translated))
	}
	return toolText("当前的热门标签:\n" + strings.Join(lines, "\n"))
}

type followIn struct {
	Restrict         string `json:"restrict,omitempty"`
	Offset           int    `json:"offset,omitempty"`
	IncludeThumbnail bool   `json:"include_thumbnail,omitempty"`
}

func (a *App) illustFollow(ctx context.Context, _ *mcp.CallToolRequest, in followIn) (*mcp.CallToolResult, textOut, error) {
	if in.Restrict == "" {
		in.Restrict = string(sdk.RestrictPublic)
	}
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return toolText(err.Error())
	}
	defer release()
	items, _, err := collectPages(ctx, offsetPlan(in.Offset), func(ctx context.Context, cursor sdk.Cursor) ([]sdk.Illust, sdk.Cursor, error) {
		result, err := client.FollowingIllusts(ctx, sdk.FollowingIllustsRequest{Restrict: sdk.Restrict(in.Restrict), Cursor: cursor})
		if err != nil {
			return nil, "", err
		}
		return result.Illusts, result.NextCursor, nil
	})
	if err != nil {
		return toolText(err.Error())
	}
	if len(items) == 0 {
		return toolText("您的关注动态中暂时没有新作品。")
	}
	return toolText(fmt.Sprintf("找到 %d 篇关注动态:\n\n%s", len(items), formatIllusts(items, in.IncludeThumbnail, in.Offset, false)))
}

func (a *App) thumbnailBase64(ctx context.Context, _ *mcp.CallToolRequest, in illustIDIn) (*mcp.CallToolResult, textOut, error) {
	client, release, err := a.openSDKOperation(ctx)
	if err != nil {
		return toolText("错误: 无法获取插画信息: " + err.Error())
	}
	defer release()
	result, err := client.IllustDetail(ctx, in.IllustID)
	if err != nil {
		return toolText("错误: 无法获取插画信息: " + err.Error())
	}
	rawURL := thumbnailURL(result.Illust)
	if rawURL == "" {
		return toolText("错误: 无法找到缩略图URL")
	}
	ref, err := client.ParseResourceRef(rawURL)
	if err != nil {
		return toolText("错误: 获取缩略图失败: " + err.Error())
	}
	response, err := client.OpenResource(ctx, sdk.OpenResourceRequest{Ref: ref})
	if err != nil {
		return toolText("错误: 获取缩略图失败: " + err.Error())
	}
	defer response.Body.Close()
	var buf strings.Builder
	writer := base64.NewEncoder(base64.StdEncoding, stringWriter{&buf})
	if _, err := io.Copy(writer, response.Body); err != nil {
		_ = writer.Close()
		return toolText("错误: 获取缩略图失败: " + err.Error())
	}
	if err := writer.Close(); err != nil {
		return toolText("错误: 获取缩略图失败: " + err.Error())
	}
	return toolText(fmt.Sprintf("缩略图数据 (插画ID: %d):\ndata:image/jpeg;base64,%s", in.IllustID, buf.String()))
}

func formatIllusts(illusts []sdk.Illust, includeThumbnail bool, offset int, ranked bool) string {
	lines := make([]string, 0, len(illusts))
	for i, illust := range illusts {
		prefix := ""
		if ranked {
			prefix = fmt.Sprintf("第 %d 名: ", i+1+offset)
		}
		lines = append(lines, prefix+formatIllust(illust, includeThumbnail))
	}
	return strings.Join(lines, "\n\n")
}

func formatIllust(illust sdk.Illust, includeThumbnail bool) string {
	tags := make([]string, 0, len(illust.Tags))
	for _, tag := range illust.Tags {
		tags = append(tags, tag.Name)
	}
	text := fmt.Sprintf("ID: %d - %q\n  作者: %s (ID: %d)\n  类型: %s\n  标签: %s\n  收藏数: %d, 浏览数: %d",
		illust.ID, illust.Title, illust.User.Name, illust.User.ID, illust.Type, strings.Join(tags, ", "), illust.TotalBookmarks, illust.TotalView)
	if includeThumbnail {
		if url := thumbnailURL(illust); url != "" {
			text += "\n  缩略图: " + url
		} else {
			text += "\n  缩略图: 暂无"
		}
	}
	return text
}

func formatUsers(users []sdk.UserPreview) string {
	lines := make([]string, 0, len(users))
	for _, preview := range users {
		user := preview.User
		followed := "未关注"
		if user.IsFollowed {
			followed = "已关注"
		}
		comment := user.Comment
		if comment == "" {
			comment = "无"
		}
		lines = append(lines, fmt.Sprintf("用户ID: %d - %s (@%s)\n  关注状态: %s\n  简介: %s", user.ID, user.Name, user.Account, followed, comment))
	}
	return strings.Join(lines, "\n\n")
}

func thumbnailURL(illust sdk.Illust) string {
	for _, value := range []string{illust.ImageURLs.SquareMedium, illust.ImageURLs.Medium} {
		if value != "" {
			return value
		}
	}
	if len(illust.MetaPages) > 0 {
		return text.FirstNonEmpty(illust.MetaPages[0].ImageURLs.SquareMedium, illust.MetaPages[0].ImageURLs.Medium)
	}
	return text.FirstNonEmpty(illust.MetaSinglePage.OriginalImageURL, illust.ImageURLs.Large)
}

type stringWriter struct {
	builder *strings.Builder
}

func (w stringWriter) Write(p []byte) (int, error) {
	return w.builder.WriteString(string(p))
}
