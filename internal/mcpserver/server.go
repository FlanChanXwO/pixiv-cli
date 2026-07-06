package mcpserver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"os"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/download"
	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/media"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/text"
	uriutil "github.com/FlanChanXwO/pixiv-cli/internal/utils/uri"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type PixivAPI interface {
	Refresh(context.Context) error
	SetRefreshToken(string)
	RefreshTokenValue() string
	UserID() int64
	IsAuthenticated() bool
	SearchIllust(context.Context, string, string, string, string, int) (*pixiv.IllustList, error)
	IllustDetail(context.Context, int64) (*pixiv.IllustDetail, error)
	IllustRelated(context.Context, int64, int) (*pixiv.IllustList, error)
	IllustRanking(context.Context, string, string, int) (*pixiv.IllustList, error)
	SearchUser(context.Context, string, int) (*pixiv.UserPreviewList, error)
	IllustRecommended(context.Context, int) (*pixiv.IllustList, error)
	TrendingTagsIllust(context.Context) (*pixiv.TrendTags, error)
	IllustFollow(context.Context, string, int) (*pixiv.IllustList, error)
	UserBookmarks(context.Context, int64, string, string, int64) (*pixiv.IllustList, error)
	UserFollowing(context.Context, int64, string, int) (*pixiv.UserPreviewList, error)
	Download(context.Context, string, io.Writer) error
}

type DownloadManager interface {
	SetDownloadPath(string) error
	Enqueue(context.Context, []int64) int
	Download(context.Context, []int64) ([]download.DownloadedArtwork, error)
}

type App struct {
	api       PixivAPI
	downloads DownloadManager
	logger    *slog.Logger
}

func New(api PixivAPI, downloads DownloadManager, logger *slog.Logger) *mcp.Server {
	app := &App{api: api, downloads: downloads, logger: logger}
	server := mcp.NewServer(&mcp.Implementation{Name: "pixiv-cli", Version: "2.0.0"}, &mcp.ServerOptions{
		Instructions: "Pixiv MCP server for searching, browsing, and downloading Pixiv content.",
	})
	app.register(server)
	return server
}

func (a *App) register(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{Name: "set_download_path", Description: "Set the default local save location for images and animations."}, a.setDownloadPath)
	mcp.AddTool(server, &mcp.Tool{Name: "download", Description: "Download one or more artworks by ID with intelligent storage rules."}, a.download)
	mcp.AddTool(server, &mcp.Tool{Name: "refresh_token", Description: "Manually refresh Pixiv API token when encountering authentication errors."}, a.refreshToken)
	mcp.AddTool(server, &mcp.Tool{Name: "set_refresh_token", Description: "Set or update the Pixiv refresh token for authentication."}, a.setRefreshToken)
	mcp.AddTool(server, &mcp.Tool{Name: "download_random_from_recommendation", Description: "Download random artworks from recommendations."}, a.downloadRandom)
	mcp.AddTool(server, &mcp.Tool{Name: "search_illust", Description: "Search for illustrations using keywords with filters."}, a.searchIllust)
	mcp.AddTool(server, &mcp.Tool{Name: "illust_detail", Description: "Get detailed information about a specific artwork."}, a.illustDetail)
	mcp.AddTool(server, &mcp.Tool{Name: "illust_related", Description: "Find artworks related to a specific illustration."}, a.illustRelated)
	mcp.AddTool(server, &mcp.Tool{Name: "illust_ranking", Description: "Browse Pixiv rankings."}, a.illustRanking)
	mcp.AddTool(server, &mcp.Tool{Name: "search_user", Description: "Search for users/artists on Pixiv."}, a.searchUser)
	mcp.AddTool(server, &mcp.Tool{Name: "illust_recommended", Description: "Get personalized artwork recommendations."}, a.illustRecommended)
	mcp.AddTool(server, &mcp.Tool{Name: "trending_tags_illust", Description: "Get currently trending illustration tags."}, a.trendingTags)
	mcp.AddTool(server, &mcp.Tool{Name: "illust_follow", Description: "Browse artworks from followed artists."}, a.illustFollow)
	mcp.AddTool(server, &mcp.Tool{Name: "user_bookmarks", Description: "Browse user's bookmarked artworks."}, a.userBookmarks)
	mcp.AddTool(server, &mcp.Tool{Name: "user_following", Description: "View user's following list."}, a.userFollowing)
	mcp.AddTool(server, &mcp.Tool{Name: "get_thumbnail_base64", Description: "Get artwork thumbnail as base64 data URL."}, a.thumbnailBase64)
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
		out := downloadOut{Delivery: deliveryLocalPath, Text: "错误：必须提供 illust_id (单个ID) 或 illust_ids (ID列表) 参数之一。"}
		return downloadResult(out), out, nil
	}
	delivery, errText := normalizeDelivery(in.Delivery)
	if errText != "" {
		out := downloadOut{Delivery: deliveryLocalPath, Text: errText}
		return downloadResult(out), out, nil
	}
	artworks, err := a.downloads.Download(ctx, ids)
	if err != nil {
		out := downloadOut{Delivery: delivery, Text: "下载失败: " + err.Error()}
		return downloadResult(out), out, nil
	}
	out, err := buildDownloadOut(delivery, artworks)
	if err != nil {
		out := downloadOut{Delivery: delivery, Text: "整理下载结果失败: " + err.Error()}
		return downloadResult(out), out, nil
	}
	result := downloadResult(out)
	if delivery == deliveryImageContent {
		for _, file := range out.Files {
			data, err := os.ReadFile(file.Path)
			if err != nil {
				out := downloadOut{Delivery: delivery, Text: "读取下载文件失败: " + err.Error()}
				return downloadResult(out), out, nil
			}
			result.Content = append(result.Content, &mcp.ImageContent{
				Data:     data,
				MIMEType: file.MIMEType,
			})
		}
	}
	return result, out, nil
}

func downloadResult(out downloadOut) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: out.Text}},
	}
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
	out := downloadOut{Delivery: delivery}
	lines := []string{fmt.Sprintf("下载完成，交付方式: %s。", delivery)}
	for _, artwork := range artworks {
		item := downloadItemOut{
			IllustID: artwork.IllustID,
			Title:    artwork.Title,
			Author:   artwork.Author,
			Type:     artwork.Type,
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
	if a.api.RefreshTokenValue() == "" {
		return toolText("错误：未设置 refresh token。请先使用 set_refresh_token 工具设置 token。")
	}
	if err := a.api.Refresh(ctx); err != nil {
		return toolText(fmt.Sprintf("Token刷新失败。可能的原因：refresh_token已过期、网络连接问题或代理设置问题。错误详情: %v", err))
	}
	return toolText(fmt.Sprintf("Token刷新成功！用户 ID: %d。现在可以正常使用Pixiv API功能了。", a.api.UserID()))
}

type setRefreshTokenIn struct {
	RefreshToken string `json:"refresh_token" jsonschema:"Pixiv refresh token"`
}

func (a *App) setRefreshToken(ctx context.Context, _ *mcp.CallToolRequest, in setRefreshTokenIn) (*mcp.CallToolResult, textOut, error) {
	token, parsedCookie := utils.ParsePixivWebRefreshTokenInput(in.RefreshToken)
	if token == "" {
		if parsedCookie {
			return toolText("错误：检测到您输入的是 Cookie 字符串，但其中没有 refresh_token。Pixiv 网页 Cookie 里的 PHPSESSID/device_token 不能直接用于 App API OAuth 刷新。请提供真正的 Pixiv refresh token，或包含 refresh_token=... 的 Cookie。")
		}
		return toolText("错误：refresh token 不能为空。")
	}
	a.api.SetRefreshToken(token)
	if err := a.api.Refresh(ctx); err != nil {
		return toolText(fmt.Sprintf("Refresh token 已在当前会话设置，但认证失败: %v\n\n请检查 token 是否有效，或稍后使用 refresh_token 工具重试认证。", err))
	}
	return toolText(fmt.Sprintf("Refresh token 已在当前会话设置并完成认证！\n用户 ID: %d\n\n现在您可以使用所有 Pixiv 功能了。", a.api.UserID()))
}

type downloadRandomIn struct {
	Count    int    `json:"count,omitempty" jsonschema:"number of random artworks to download"`
	Delivery string `json:"delivery,omitempty" jsonschema:"delivery mode: local_path or image_content"`
}

func (a *App) downloadRandom(ctx context.Context, req *mcp.CallToolRequest, in downloadRandomIn) (*mcp.CallToolResult, downloadOut, error) {
	if err := a.ensureAuth(ctx); err != nil {
		return nil, downloadOut{Delivery: deliveryLocalPath, Text: err.Error()}, nil
	}
	count := in.Count
	if count <= 0 {
		count = 5
	}
	if count > 20 {
		count = 20
	}
	result, err := a.api.IllustRecommended(ctx, 0)
	if err != nil {
		return nil, downloadOut{Delivery: deliveryLocalPath, Text: "获取推荐列表失败: " + err.Error()}, nil
	}
	if len(result.Illusts) == 0 {
		return nil, downloadOut{Delivery: deliveryLocalPath, Text: "无法获取推荐内容，列表为空。"}, nil
	}
	if count > len(result.Illusts) {
		count = len(result.Illusts)
	}
	rand.Shuffle(len(result.Illusts), func(i, j int) { result.Illusts[i], result.Illusts[j] = result.Illusts[j], result.Illusts[i] })
	ids := make([]int64, 0, count)
	for _, illust := range result.Illusts[:count] {
		ids = append(ids, illust.ID)
	}
	return a.download(ctx, req, downloadIn{IllustIDs: ids, Delivery: in.Delivery})
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
		in.SearchTarget = string(pixiv.SearchTargetPartialMatchForTags)
	}
	if in.Sort == "" {
		in.Sort = string(pixiv.SortModeDateDesc)
	}
	word := in.Word
	if in.SearchR18 {
		word += " R-18"
	}
	result, err := a.api.SearchIllust(ctx, word, in.SearchTarget, in.Sort, in.Duration, in.Offset)
	if err != nil {
		return toolText(err.Error())
	}
	if len(result.Illusts) == 0 {
		return toolText(fmt.Sprintf("抱歉，根据您提供的关键词 '%s'，未能找到相关的插画。", word))
	}
	return toolText(fmt.Sprintf("找到 %d 张关于 '%s' 的插画:\n\n%s", len(result.Illusts), word, formatIllusts(result.Illusts, in.IncludeThumbnail, in.Offset, false)))
}

type illustIDIn struct {
	IllustID int64 `json:"illust_id"`
}

func (a *App) illustDetail(ctx context.Context, _ *mcp.CallToolRequest, in illustIDIn) (*mcp.CallToolResult, textOut, error) {
	result, err := a.api.IllustDetail(ctx, in.IllustID)
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
	result, err := a.api.IllustRelated(ctx, in.IllustID, in.Offset)
	if err != nil {
		return toolText(err.Error())
	}
	if len(result.Illusts) == 0 {
		return toolText(fmt.Sprintf("找不到与插画 %d 相关的推荐。", in.IllustID))
	}
	return toolText(fmt.Sprintf("找到 %d 张相关推荐:\n\n%s", len(result.Illusts), formatIllusts(result.Illusts, in.IncludeThumbnail, in.Offset, false)))
}

type rankingIn struct {
	Mode             string `json:"mode,omitempty"`
	Date             string `json:"date,omitempty"`
	Offset           int    `json:"offset,omitempty"`
	IncludeThumbnail bool   `json:"include_thumbnail,omitempty"`
}

func (a *App) illustRanking(ctx context.Context, _ *mcp.CallToolRequest, in rankingIn) (*mcp.CallToolResult, textOut, error) {
	if in.Mode == "" {
		in.Mode = string(pixiv.RankingModeDay)
	}
	result, err := a.api.IllustRanking(ctx, in.Mode, in.Date, in.Offset)
	if err != nil {
		return toolText(err.Error())
	}
	if len(result.Illusts) == 0 {
		return toolText(fmt.Sprintf("找不到模式为 '%s' 的排行榜结果。", in.Mode))
	}
	return toolText(fmt.Sprintf("%s 排行榜:\n\n%s", strings.Title(in.Mode), formatIllusts(result.Illusts, in.IncludeThumbnail, in.Offset, true)))
}

type searchUserIn struct {
	Word   string `json:"word"`
	Offset int    `json:"offset,omitempty"`
}

func (a *App) searchUser(ctx context.Context, _ *mcp.CallToolRequest, in searchUserIn) (*mcp.CallToolResult, textOut, error) {
	result, err := a.api.SearchUser(ctx, in.Word, in.Offset)
	if err != nil {
		return toolText(err.Error())
	}
	if len(result.UserPreviews) == 0 {
		return toolText(fmt.Sprintf("抱歉，未能找到名为 '%s' 的用户。", in.Word))
	}
	return toolText(fmt.Sprintf("找到 %d 位用户:\n\n%s", len(result.UserPreviews), formatUsers(result.UserPreviews)))
}

type offsetThumbnailIn struct {
	Offset           int  `json:"offset,omitempty"`
	IncludeThumbnail bool `json:"include_thumbnail,omitempty"`
}

func (a *App) illustRecommended(ctx context.Context, _ *mcp.CallToolRequest, in offsetThumbnailIn) (*mcp.CallToolResult, textOut, error) {
	if err := a.ensureAuth(ctx); err != nil {
		return toolText(err.Error())
	}
	result, err := a.api.IllustRecommended(ctx, in.Offset)
	if err != nil {
		return toolText(err.Error())
	}
	if len(result.Illusts) == 0 {
		return toolText("暂无推荐内容。")
	}
	return toolText(fmt.Sprintf("为您推荐 %d 张插画:\n\n%s", len(result.Illusts), formatIllusts(result.Illusts, in.IncludeThumbnail, in.Offset, false)))
}

func (a *App) trendingTags(ctx context.Context, _ *mcp.CallToolRequest, _ emptyIn) (*mcp.CallToolResult, textOut, error) {
	result, err := a.api.TrendingTagsIllust(ctx)
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
	if err := a.ensureAuth(ctx); err != nil {
		return toolText(err.Error())
	}
	if in.Restrict == "" {
		in.Restrict = string(pixiv.RestrictPublic)
	}
	result, err := a.api.IllustFollow(ctx, in.Restrict, in.Offset)
	if err != nil {
		return toolText(err.Error())
	}
	if len(result.Illusts) == 0 {
		return toolText("您的关注动态中暂时没有新作品。")
	}
	return toolText(fmt.Sprintf("找到 %d 篇关注动态:\n\n%s", len(result.Illusts), formatIllusts(result.Illusts, in.IncludeThumbnail, in.Offset, false)))
}

type bookmarksIn struct {
	UserIDToCheck int64  `json:"user_id_to_check,omitempty"`
	Restrict      string `json:"restrict,omitempty"`
	Tag           string `json:"tag,omitempty"`
	MaxBookmarkID int64  `json:"max_bookmark_id,omitempty"`
}

func (a *App) userBookmarks(ctx context.Context, _ *mcp.CallToolRequest, in bookmarksIn) (*mcp.CallToolResult, textOut, error) {
	if err := a.ensureAuth(ctx); err != nil {
		return toolText(err.Error())
	}
	if in.Restrict == "" {
		in.Restrict = string(pixiv.RestrictPublic)
	}
	userID := in.UserIDToCheck
	if userID == 0 {
		userID = a.api.UserID()
	}
	if userID == 0 {
		return toolText("错误: 查询自己的收藏时，需要先认证以获取用户ID。")
	}
	result, err := a.api.UserBookmarks(ctx, userID, in.Restrict, in.Tag, in.MaxBookmarkID)
	if err != nil {
		return toolText(err.Error())
	}
	if len(result.Illusts) == 0 {
		return toolText(fmt.Sprintf("找不到用户 %d 的收藏。", userID))
	}
	return toolText(fmt.Sprintf("找到用户 %d 的 %d 个收藏:\n\n%s", userID, len(result.Illusts), formatIllusts(result.Illusts, false, 0, false)))
}

type followingIn struct {
	UserIDToCheck int64  `json:"user_id_to_check,omitempty"`
	Restrict      string `json:"restrict,omitempty"`
	Offset        int    `json:"offset,omitempty"`
}

func (a *App) userFollowing(ctx context.Context, _ *mcp.CallToolRequest, in followingIn) (*mcp.CallToolResult, textOut, error) {
	if err := a.ensureAuth(ctx); err != nil {
		return toolText(err.Error())
	}
	if in.Restrict == "" {
		in.Restrict = string(pixiv.RestrictPublic)
	}
	userID := in.UserIDToCheck
	if userID == 0 {
		userID = a.api.UserID()
	}
	if userID == 0 {
		return toolText("错误: 查询自己的关注列表时，需要先认证以获取用户ID。")
	}
	result, err := a.api.UserFollowing(ctx, userID, in.Restrict, in.Offset)
	if err != nil {
		return toolText(err.Error())
	}
	if len(result.UserPreviews) == 0 {
		return toolText(fmt.Sprintf("用户 %d 没有关注任何人。", userID))
	}
	return toolText(fmt.Sprintf("用户 %d 关注了 %d 位用户:\n\n%s", userID, len(result.UserPreviews), formatUsers(result.UserPreviews)))
}

func (a *App) thumbnailBase64(ctx context.Context, _ *mcp.CallToolRequest, in illustIDIn) (*mcp.CallToolResult, textOut, error) {
	result, err := a.api.IllustDetail(ctx, in.IllustID)
	if err != nil {
		return toolText("错误: 无法获取插画信息: " + err.Error())
	}
	rawURL := thumbnailURL(result.Illust)
	if rawURL == "" {
		return toolText("错误: 无法找到缩略图URL")
	}
	var buf strings.Builder
	writer := base64.NewEncoder(base64.StdEncoding, stringWriter{&buf})
	if err := a.api.Download(ctx, rawURL, writer); err != nil {
		_ = writer.Close()
		return toolText("错误: 获取缩略图失败: " + err.Error())
	}
	_ = writer.Close()
	return toolText(fmt.Sprintf("缩略图数据 (插画ID: %d):\ndata:image/jpeg;base64,%s", in.IllustID, buf.String()))
}

func (a *App) ensureAuth(ctx context.Context) error {
	if a.api.IsAuthenticated() {
		return nil
	}
	if a.api.RefreshTokenValue() == "" {
		return fmt.Errorf("错误: 此功能需要认证。请先使用 set_refresh_token 工具或在客户端设置 PIXIV_REFRESH_TOKEN 环境变量。")
	}
	if err := a.api.Refresh(ctx); err != nil {
		return fmt.Errorf("错误: 自动认证失败: %v", err)
	}
	return nil
}

func formatIllusts(illusts []pixiv.Illust, includeThumbnail bool, offset int, ranked bool) string {
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

func formatIllust(illust pixiv.Illust, includeThumbnail bool) string {
	tags := make([]string, 0, min(len(illust.Tags), 5))
	for _, tag := range illust.Tags {
		if len(tags) == 5 {
			break
		}
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

func formatUsers(users []pixiv.UserPreview) string {
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

func thumbnailURL(illust pixiv.Illust) string {
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
