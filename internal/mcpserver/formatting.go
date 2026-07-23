package mcpserver

import (
	"fmt"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/utils/text"
	sdk "github.com/FlanChanXwO/pixiv-cli/pixiv"
)

func formatIllusts(illusts []sdk.Illust, offset int, ranked bool) string {
	lines := make([]string, 0, len(illusts))
	for i, illust := range illusts {
		prefix := ""
		if ranked {
			prefix = fmt.Sprintf("Rank %d: ", i+1+offset)
		}
		lines = append(lines, prefix+formatIllust(illust))
	}
	return strings.Join(lines, "\n\n")
}

func formatIllust(illust sdk.Illust) string {
	tags := make([]string, 0, len(illust.Tags))
	for _, tag := range illust.Tags {
		tags = append(tags, tag.Name)
	}
	// 作品页 URL 固定放在每件作品文本的第一行，便于 Agent/人类直接打开。
	url := illust.URL
	if url == "" && illust.ID > 0 {
		url = fmt.Sprintf("https://www.pixiv.net/artworks/%d", illust.ID)
	}
	return fmt.Sprintf("%s\nID: %d - %q\n  Author: %s (ID: %d)\n  Type: %s\n  Tags: %s\n  Bookmarks: %d, Views: %d",
		url, illust.ID, illust.Title, illust.User.Name, illust.User.ID, illust.Type, strings.Join(tags, ", "), illust.TotalBookmarks, illust.TotalView)
}

func formatUsers(users []sdk.UserPreview) string {
	lines := make([]string, 0, len(users))
	for _, preview := range users {
		user := preview.User
		followed := "not following"
		if user.IsFollowed {
			followed = "following"
		}
		comment := user.Comment
		if comment == "" {
			comment = "none"
		}
		lines = append(lines, fmt.Sprintf("User ID: %d - %s (@%s)\n  Follow status: %s\n  Bio: %s", user.ID, user.Name, user.Account, followed, comment))
	}
	return strings.Join(lines, "\n\n")
}

func formatNovels(novels []sdk.Novel) string {
	lines := make([]string, 0, len(novels))
	for _, novel := range novels {
		url := novel.URL
		if url == "" && novel.ID > 0 {
			url = fmt.Sprintf("https://www.pixiv.net/novel/show.php?id=%d", novel.ID)
		}
		lines = append(lines, fmt.Sprintf("%s\nID: %d - %q\n  Author: %s (ID: %d)\n  Text length: %d\n  Original: %t",
			url, novel.ID, novel.Title, novel.User.Name, novel.User.ID, novel.TextLength, novel.IsOriginal))
	}
	return strings.Join(lines, "\n\n")
}

// formatSDKIllusts/formatSDKUsers 延续旧 MCP 中文文本结果，同时 structured content 提供
// 面向调用方的规范化 SDK 模型。
func formatSDKIllusts(illusts []sdk.Illust) string {
	lines := make([]string, 0, len(illusts))
	for _, illust := range illusts {
		lines = append(lines, formatIllust(illust))
	}
	return strings.Join(lines, "\n\n")
}

func formatSDKUsers(users []sdk.UserPreview) string {
	lines := make([]string, 0, len(users))
	for _, preview := range users {
		user := preview.User
		followed := "not following"
		if user.IsFollowed {
			followed = "following"
		}
		comment := user.Comment
		if comment == "" {
			comment = "none"
		}
		lines = append(lines, fmt.Sprintf("User ID: %d - %s (@%s)\n  Follow status: %s\n  Bio: %s", user.ID, user.Name, user.Account, followed, comment))
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
